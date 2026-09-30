# syntax=docker/dockerfile:1.7
#
# Build optimisation notes:
#  - BuildKit cache mount on /go/pkg/mod persists module downloads between
#    builds → go mod download goes from ~44 s to ~0 s on warm cache.
#  - BuildKit cache mount on /root/.cache/go-build persists compiled packages
#    → go build goes from ~970 s to ~30-60 s on warm cache when only a few
#    files changed (compiler replays cached .a packages for unchanged deps).
#  - CGO_ENABLED=0 keeps the binary fully static so it runs in distroless.
#  - GOARCH=amd64 is explicit: the server is x86-64; avoids any arm emulation.
FROM golang:1.27.0 AS build
WORKDIR /app
COPY go.mod go.sum* ./
RUN --mount=type=cache,id=mindstrata-api-gomod,target=/go/pkg/mod,sharing=locked \
    go mod download
COPY . .
RUN --mount=type=cache,id=mindstrata-api-gomod,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,id=mindstrata-api-gobuild,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/worker ./cmd/worker

FROM gcr.io/distroless/base-debian12
WORKDIR /app
COPY --from=build /out/api /app/api
COPY --from=build /out/worker /worker
EXPOSE 8080
CMD ["/app/api"]
