#!/bin/bash
# Smoke test: full pipeline without API key (no LLM)
# Usage: bash scripts/smoke-test.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
cd "$PROJECT_DIR"

BIN="$PROJECT_DIR/daily-report-daemon"
if [ ! -f "$BIN" ]; then
    echo "Building..."
    go build -o "$BIN" ./cmd/daily-report-daemon
fi

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

echo "=== Smoke Test: daily-report-daemon Phase 0 ==="
echo ""

# Setup test repo
cd "$TMPDIR"
git init -q
git config user.email "smoke@test.com"
git config user.name "smoke"
echo "# Smoke Test Project" > README.md
echo "package main" > main.go
git add . && git commit -q -m "init"

echo "1. Init"
$BIN init -w "$TMPDIR"
echo ""

echo "2. Scan (--no-llm)"
$BIN scan -w "$TMPDIR" --no-llm
echo ""

echo "3. Agent context"
$BIN agent-context -w "$TMPDIR"
echo ""

# Verify outputs
echo "4. Verifying outputs..."

EVIDENCE_COUNT=$(find "$TMPDIR/.daily-report-daemon/runs" -name "evidence.jsonl" | wc -l)
if [ "$EVIDENCE_COUNT" -eq 0 ]; then
    echo "FAIL: no evidence.jsonl generated"
    exit 1
fi

CONTEXT_FILE="$TMPDIR/.daily-report-daemon/context/AGENTS.generated.md"
if [ ! -f "$CONTEXT_FILE" ]; then
    echo "FAIL: AGENTS.generated.md not found"
    exit 1
fi

if ! grep -q "Project Overview" "$CONTEXT_FILE"; then
    echo "FAIL: AGENTS.generated.md missing Project Overview section"
    exit 1
fi

echo ""
echo "=== Smoke Test PASSED ==="
echo "Evidence files: $EVIDENCE_COUNT"
echo "Agent context: $CONTEXT_FILE"
