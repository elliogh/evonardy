#!/usr/bin/env bash
set -euo pipefail
mkdir -p .cache/smoke-training
go run ./cmd/evonardy train --config configs/ga-linear-smoke.json --data-dir .cache/smoke-training --save-name "GA smoke" > .cache/smoke-training-result.json
cat .cache/smoke-training-result.json
bot_id=$(node -p 'JSON.parse(require("node:fs").readFileSync(".cache/smoke-training-result.json", "utf8")).saved_bot.id')
go run ./cmd/evonardy evaluate --config configs/evaluation-smoke.json --bot "$bot_id" --data-dir .cache/smoke-training

for method in td0 td-lambda; do
  td_config=configs/td-zero-smoke.json
  if [[ "$method" == td-lambda ]]; then
    td_config=configs/td-lambda-smoke.json
  fi
  td_result=".cache/smoke-training-${method}.json"
  go run ./cmd/evonardy train --algorithm "$method" --config "$td_config" --data-dir .cache/smoke-training --save-name "$method smoke" > "$td_result"
  cat "$td_result"
  bot_id=$(node -e 'const x = JSON.parse(require("node:fs").readFileSync(process.argv[1], "utf8")); if (x.state !== "completed" || x.counters.games !== 4 || x.counters.updates <= 0 || x.counters.updates !== x.counters.decisions || x.saved_bot.kind !== "tanh-56-32-1-v1") throw new Error("incomplete neural smoke"); process.stdout.write(x.saved_bot.id)' "$td_result")
  go run ./cmd/evonardy evaluate --config configs/evaluation-smoke.json --bot "$bot_id" --data-dir .cache/smoke-training
done
