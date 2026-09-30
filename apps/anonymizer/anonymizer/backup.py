"""Offsite DB backup activity: pg_dump -> gzip -> S3, with rotation.

The DatabaseBackupWorkflow lives in the Go worker (apps/api/internal/dbbackup);
only the backup_database activity runs here, because this image has pg_dump
(postgresql-client-17) and boto3.
"""

import asyncio
import dataclasses
import datetime as dt
import os

import boto3
from temporalio import activity

# A normal gzip dump of prod is ~9 MB; under a megabyte is almost surely an
# empty DB or an error, so it is neither uploaded nor rotated.
MIN_DUMP_BYTES = 1_000_000


@dataclasses.dataclass
class BackupResult:
    key: str
    size_bytes: int
    kept: list[str]
    deleted: list[str]


def _heartbeat(details) -> None:
    try:
        activity.heartbeat(details)
    except RuntimeError:
        pass


def keys_to_delete(keys: list[str], keep: int) -> list[str]:
    """Keys to delete so that only the newest `keep` remain.

    Names carry a UTC timestamp, so lexical order is chronological.
    """
    if keep <= 0 or len(keys) <= keep:
        return []
    return sorted(keys)[:-keep]


def s3_client():
    return boto3.client(
        "s3",
        endpoint_url=os.environ["BACKUP_S3_ENDPOINT"],
        region_name=os.environ.get("BACKUP_S3_REGION", "ru-1"),
        aws_access_key_id=os.environ["BACKUP_S3_ACCESS_KEY"],
        aws_secret_access_key=os.environ["BACKUP_S3_SECRET_KEY"],
    )


def list_backup_keys(s3, bucket: str, prefix: str) -> list[str]:
    response = s3.list_objects_v2(Bucket=bucket, Prefix=prefix + "/")
    return [item["Key"] for item in response.get("Contents", [])]


def prune(s3, bucket: str, prefix: str, keep: int) -> tuple[list[str], list[str]]:
    """Delete old backups; return (kept, deleted)."""
    keys = list_backup_keys(s3, bucket, prefix)
    doomed = keys_to_delete(keys, keep)
    for key in doomed:
        s3.delete_object(Bucket=bucket, Key=key)
    kept = [key for key in keys if key not in doomed]
    return kept, doomed


async def _run_pg_dump(db_url: str, path: str) -> None:
    proc = await asyncio.create_subprocess_exec(
        "pg_dump",
        "--dbname=" + db_url,
        "--format=plain",
        "--compress=6",
        "--file=" + path,
        stderr=asyncio.subprocess.PIPE,
    )

    # Drain stderr in the background so pg_dump never blocks on a full pipe.
    stderr_chunks: list[bytes] = []

    async def drain() -> None:
        while True:
            chunk = await proc.stderr.read(4096)
            if not chunk:
                return
            stderr_chunks.append(chunk)

    drain_task = asyncio.create_task(drain())

    # The dump takes tens of seconds; heartbeat so Temporal knows we are alive.
    while True:
        try:
            await asyncio.wait_for(asyncio.shield(proc.wait()), timeout=10)
            break
        except asyncio.TimeoutError:
            _heartbeat("pg_dump running")
    await drain_task

    if proc.returncode != 0:
        stderr_text = b"".join(stderr_chunks).decode(errors="replace")
        raise RuntimeError(f"pg_dump failed (code {proc.returncode}): {stderr_text[:2000]}")


@activity.defn(name="backup_database")
async def backup_database() -> BackupResult:
    db_url = os.environ["BACKUP_DATABASE_URL"]
    bucket = os.environ["BACKUP_S3_BUCKET"]
    prefix = os.environ.get("BACKUP_S3_PREFIX", "db-backups")
    keep = int(os.environ.get("BACKUP_KEEP", "3"))

    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%d-%H%M%S")
    key = f"{prefix}/mindstrata-{stamp}.sql.gz"
    path = f"/tmp/mindstrata-{stamp}.sql.gz"

    try:
        await _run_pg_dump(db_url, path)

        size = os.path.getsize(path)
        if size < MIN_DUMP_BYTES:
            raise RuntimeError(f"dump suspiciously small: {size} bytes — not uploading")

        s3 = s3_client()
        _heartbeat("uploading")
        await asyncio.to_thread(s3.upload_file, path, bucket, key)
        kept, deleted = await asyncio.to_thread(prune, s3, bucket, prefix, keep)
        return BackupResult(key=key, size_bytes=size, kept=kept, deleted=deleted)
    finally:
        if os.path.exists(path):
            os.remove(path)
