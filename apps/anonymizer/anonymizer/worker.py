"""Temporal worker: the monthly irreversible pass and the offsite DB backup."""

from __future__ import annotations

import asyncio
import os

from temporalio.client import Client
from temporalio.worker import Worker

from . import ner
from .activities import anonymize_texts
from .backup import backup_database


async def main() -> None:
    ner.warm_up()
    client = await Client.connect(
        os.environ.get("TEMPORAL_ADDRESS", "temporal-server:7233"),
        namespace=os.environ.get("TEMPORAL_NAMESPACE", "default"),
    )
    worker = Worker(
        client,
        task_queue=os.environ.get("TASK_QUEUE", "anonymizer-python"),
        activities=[anonymize_texts, backup_database],
    )
    await worker.run()


if __name__ == "__main__":
    asyncio.run(main())
