"""Rotation of offsite backups (anonymizer/backup.py)."""

from anonymizer.backup import keys_to_delete, prune


def test_keys_to_delete_fewer_than_keep():
    assert keys_to_delete(["a", "b"], keep=3) == []


def test_keys_to_delete_exactly_keep():
    assert keys_to_delete(["a", "b", "c"], keep=3) == []


def test_keys_to_delete_removes_oldest():
    keys = [
        "db-backups/mindstrata-20260601-043000.sql.gz",
        "db-backups/mindstrata-20260524-043000.sql.gz",
        "db-backups/mindstrata-20260614-043000.sql.gz",
        "db-backups/mindstrata-20260517-043000.sql.gz",
        "db-backups/mindstrata-20260607-043000.sql.gz",
    ]
    assert keys_to_delete(keys, keep=3) == [
        "db-backups/mindstrata-20260517-043000.sql.gz",
        "db-backups/mindstrata-20260524-043000.sql.gz",
    ]


def test_keys_to_delete_keep_zero_is_noop():
    # keep<=0 guards against a config typo: better to delete nothing than
    # every backup
    assert keys_to_delete(["a", "b"], keep=0) == []


class FakeS3:
    def __init__(self, keys):
        self.keys = list(keys)
        self.deleted = []

    def list_objects_v2(self, Bucket, Prefix):
        matching = [k for k in self.keys if k.startswith(Prefix)]
        return {"Contents": [{"Key": k} for k in matching]} if matching else {}

    def delete_object(self, Bucket, Key):
        self.deleted.append(Key)
        self.keys.remove(Key)


def test_prune_deletes_oldest_and_reports():
    s3 = FakeS3(
        [
            "db-backups/mindstrata-20260517-043000.sql.gz",
            "db-backups/mindstrata-20260524-043000.sql.gz",
            "db-backups/mindstrata-20260601-043000.sql.gz",
            "db-backups/mindstrata-20260607-043000.sql.gz",
            "other-folder/unrelated.txt",
        ]
    )
    kept, deleted = prune(s3, "bucket", "db-backups", keep=3)

    assert deleted == ["db-backups/mindstrata-20260517-043000.sql.gz"]
    assert s3.deleted == deleted
    assert kept == [
        "db-backups/mindstrata-20260524-043000.sql.gz",
        "db-backups/mindstrata-20260601-043000.sql.gz",
        "db-backups/mindstrata-20260607-043000.sql.gz",
    ]
    # Foreign files in the bucket are left alone
    assert "other-folder/unrelated.txt" in s3.keys


def test_prune_empty_bucket():
    s3 = FakeS3([])
    kept, deleted = prune(s3, "bucket", "db-backups", keep=3)
    assert kept == []
    assert deleted == []
