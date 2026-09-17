# Contributing

## Humans

1. Read the [README](README.md) for how to run and play.
2. Pack authors: play a game and type `help building`, or read `internal/docs/engine/building.md`.
3. Engine changes: read [AGENTS.md](AGENTS.md), then `go test ./...`.

## Agents

Follow [AGENTS.md](AGENTS.md). It is the source of truth for architecture, locked decisions, and where files live.

## Checks before a PR

```bash
go test ./...
go vet ./...
go build -o sudengine ./cmd/sudengine
```

Do not commit the `sudengine` binary or `saves/`.
