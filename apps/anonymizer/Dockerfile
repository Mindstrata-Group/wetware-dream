FROM python:3.12-slim AS base
WORKDIR /app
# pg_dump 17 for the backup activity: it must not be older than the server
# (postgres:17), and trixie ships it in the main repositories.
RUN apt-get update && \
    apt-get install -y --no-install-recommends postgresql-client-17 && \
    rm -rf /var/lib/apt/lists/*
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY anonymizer ./anonymizer
RUN useradd --system --uid 10001 anonymizer
USER anonymizer

FROM base AS test
USER root
COPY requirements-dev.txt pytest.ini ./
RUN pip install --no-cache-dir -r requirements-dev.txt
COPY tests ./tests
USER anonymizer

# One image, two processes: the Temporal worker (default) and the HTTP API
# of the reversible mode (command: python -m anonymizer.server).
FROM base AS production
EXPOSE 8090
CMD ["python", "-m", "anonymizer.worker"]
