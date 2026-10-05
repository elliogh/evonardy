#!/usr/bin/env bash
set -euo pipefail
mkdir -p .cache/smoke-training
go run ./cmd/evonardy train --config configs/ga-linear-smoke.json --data-dir .cache/smoke-training --save-name "GA smoke" > .cache/smoke-training-result.json
cat .cache/smoke-training-result.json
bot_id=$(node -p 'JSON.parse(require("node:fs").readFileSync(".cache/smoke-training-result.json", "utf8")).saved_bot.id')
go run ./cmd/evonardy evaluate --config configs/evaluation-smoke.json --bot "$bot_id" --data-dir .cache/smoke-training
