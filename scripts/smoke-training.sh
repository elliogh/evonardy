#!/usr/bin/env bash
set -euo pipefail
mkdir -p .cache/smoke-training
go run ./cmd/evonardy train --config configs/ga-linear-smoke.json --data-dir .cache/smoke-training --save-name "GA smoke" > .cache/smoke-training-result.json
cat .cache/smoke-training-result.json
bot_id=$(node -p 'JSON.parse(require("node:fs").readFileSync(".cache/smoke-training-result.json", "utf8")).saved_bot.id')
go run ./cmd/evonardy evaluate --config configs/evaluation-smoke.json --bot "$bot_id" --data-dir .cache/smoke-training

source_bot_id="$bot_id"
source_result=".cache/smoke-training-from-model.json"
go run ./cmd/evonardy train --config configs/ga-linear-smoke.json --source-bot "$source_bot_id" --data-dir .cache/smoke-training --save-name "Saved-source smoke" > "$source_result"
cat "$source_result"
bot_id=$(node -e 'const fs=require("node:fs"),crypto=require("node:crypto"); const x=JSON.parse(fs.readFileSync(process.argv[1],"utf8")),id=process.argv[2]; const sha=crypto.createHash("sha256").update(fs.readFileSync(`.cache/smoke-training/bots/${id}/model.json`)).digest("hex"); if(x.state!=="completed" || x.algorithm!=="ga-linear-from-model-v1" || x.source_bot_id!==id || x.source_model_sha256!==sha || x.counters.games!==48 || x.generation_budget!==24 || x.saved_bot.kind!=="linear") throw Error("incomplete saved-source smoke or changed source"); process.stdout.write(x.saved_bot.id)' "$source_result" "$source_bot_id")
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

for method in ga-mlp hybrid; do
  population_result=".cache/smoke-training-${method}.json"
  go run ./cmd/evonardy train --algorithm "$method" --config "configs/${method}-smoke.json" --data-dir .cache/smoke-training --save-name "$method smoke" > "$population_result"
  cat "$population_result"
  bot_id=$(node -e 'const x=JSON.parse(require("node:fs").readFileSync(process.argv[1], "utf8")); const hybrid=process.argv[2]==="hybrid"; if(x.state!=="completed" || x.counters.games!==(hybrid?80:32) || x.population_progress.selection_games!==(hybrid?64:32) || x.population_progress.training_games!==(hybrid?16:0) || x.counters.crossovers!==0 || (hybrid ? x.counters.updates<=0 : (x.counters.updates||0)!==0) || x.saved_bot.kind!=="tanh-56-32-1-v1") throw Error("incomplete population smoke"); process.stdout.write(x.saved_bot.id)' "$population_result" "$method")
  go run ./cmd/evonardy evaluate --config configs/evaluation-smoke.json --bot "$bot_id" --data-dir .cache/smoke-training
done
