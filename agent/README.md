# Agent Workflow & Planning Guide

This directory is an agent-friendly knowledge base designed to give coding agents (and humans pairing with them) immediate context on project history, ongoing tasks, and planned feature architectures.

## Structure

* **[`progress.md`](progress.md)**: Chronological log of completed milestones, recent commits, and the current functional state of the engine. Consult this first when resuming work.
* **[`roadmap.md`](roadmap.md)**: Prioritized task backlog with statuses (`[ ]` todo, `[-]` in progress, `[x]` done).
* **[`features/`](features/)**: Detailed design documents and architecture proposals for major upcoming systems:
  * **[`features/weather.md`](features/weather.md)**: Implemented clock and weather scales, the pack schema, and room flags.
  * **[`features/economy.md`](features/economy.md)**: Faucet/sink balance, dynamic merchant inventory/purses, item durability/repair sinks, and crafting pipelines.

## Instructions for Agents

1. **Before Starting**:
   * Read [`AGENTS.md`](../AGENTS.md) in the repository root for immutable engine decisions and boundaries.
   * Check [`progress.md`](progress.md) to see where the previous session ended.
   * Check [`roadmap.md`](roadmap.md) to select or confirm the active task.
2. **During Development**:
   * Adhere to content vs. engine separation: game fiction, room descriptions, and pack stats belong in packs (`games/<id>/`), while generic mechanics belong in `internal/`.
   * Run tests frequently: `go test ./...`.
3. **When Finishing a Task / Session**:
   * Update [`roadmap.md`](roadmap.md) to reflect completed or modified tasks.
   * Add an entry in [`progress.md`](progress.md) summarizing changes made, tests passing, and next suggested steps.
