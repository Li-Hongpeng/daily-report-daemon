# daily-report-daemon

A local-first dev activity observer & report generator.

Phase 0 (current): CLI prototype for single Git repository.

## Quick Start

```bash
# Build
go build -o daily-report-daemon ./cmd/daily-report-daemon

# Version check
./daily-report-daemon --version

# Help
./daily-report-daemon help
```

## Dry-Run (No API Key Required)

You can run the full pipeline without an LLM API key:

```bash
# Initialize a workspace
./daily-report-daemon init -w /path/to/your/repo

# Scan only (no LLM)
./daily-report-daemon scan -w /path/to/your/repo --no-llm

# Generate agent context from scan results
./daily-report-daemon agent-context -w /path/to/your/repo

# Or use dry-run to see what would be sent to the LLM
./daily-report-daemon scan -w /path/to/your/repo --dry-run
```

## Full Pipeline (Requires API Key)

```bash
export OPENAI_API_KEY=sk-...

# One-shot: scan + report + agent-context
./daily-report-daemon run -w /path/to/your/repo
```

## Smoke Test

```bash
bash scripts/smoke-test.sh
```

## Requirements

- Go 1.19+
- Git (for workspace scanning)
- OpenAI-compatible API endpoint (for LLM-based report generation)

## Project Status

Phase 0 – CLI prototype. Manual trigger only; no background daemon.

## Documentation

- [PRD](./docs/PRD-v1.md)
- [Technical Architecture](./docs/TECHNICAL-ARCHITECTURE.md)
- [Phase 0 Task List](./docs/PHASE-0-TASKS.md)

## Examples

- [Sample Evidence (JSONL)](./examples/evidence.jsonl)
- [Sample Project Metadata](./examples/project-metadata.json)
- [Sample Daily Report](./examples/sample-report.md)
- [Sample AGENTS.generated.md](./examples/sample-agents-generated.md)
