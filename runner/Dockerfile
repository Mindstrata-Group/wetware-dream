# GitHub Actions runner for a contributor's fork.
#
# Based on GitHub's official image (ghcr.io/actions/actions-runner) with a
# pinned version. On top of it only what our checks cannot build without:
# make (make check), gcc (go test -race needs cgo) and git.
ARG RUNNER_VERSION=2.337.0
FROM ghcr.io/actions/actions-runner:${RUNNER_VERSION}

USER root
RUN apt-get update \
 && apt-get install -y --no-install-recommends make gcc libc6-dev git ca-certificates \
 && rm -rf /var/lib/apt/lists/*

COPY --chmod=755 entrypoint.sh /home/runner/mindstrata-entrypoint.sh

USER runner
WORKDIR /home/runner
ENTRYPOINT ["/home/runner/mindstrata-entrypoint.sh"]
