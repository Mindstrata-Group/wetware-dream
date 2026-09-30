# Changelog

All notable changes are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and the project uses
[Semantic Versioning](https://semver.org/): MAJOR for incompatible changes to
the API, the database or the deploy template, MINOR for new features, PATCH
for fixes. A release is a tag `vX.Y.Z`; the release workflow publishes
images and the section of this file for that version.

## [Unreleased]

## [0.1.0] - YYYY-MM-DD — "Flying Sheep"

### Added

- First public release: Go API, Next.js web app, MCP admin server.
- Personal-data anonymizer (`apps/anonymizer`): reversible masking before an
  external AI provider and an irreversible monthly pass.
- One-command local run (`docker compose up`) and production template
  (`deploy/`) with automatic HTTPS.
- Repository gates in Go (`tools/prcheck`), contributor runner (`runner/`).
