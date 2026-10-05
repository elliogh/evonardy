#!/usr/bin/env bash
set -euo pipefail
# Keep the actual bounded artifacts for inspection; ordinary verification never
# trains enough games to establish method superiority or champion confirmation.
mkdir -p .cache
research_dir=$(mktemp -d .cache/smoke-research.XXXXXX)
go run ./cmd/evonardy experiment --config configs/research-smoke.json --data-dir "$research_dir" > "$research_dir/result.json"
node - "$research_dir" <<'JS'
const fs = require("node:fs");
const crypto = require("node:crypto");
const dir = process.argv[2];
const x = JSON.parse(fs.readFileSync(`${dir}/result.json`, "utf8"));
if (x.state !== "completed" || x.runs.length !== 6 || x.methods.length !== 3 || x.differences.length !== 3 || x.counters.games !== 130 || x.counters.completed_games !== 130 || x.counters.truncated_games !== 0 || !x.selection_locked || x.verdict.status !== "candidate" || x.final_counters.games !== 4) throw Error("incomplete bounded research");
for (const method of x.methods) if (method.estimate.training_seeds !== 2 || method.estimate.pairs !== 2) throw Error("missing seed variability");
for (const run of x.runs) if (!run.bot_id || run.manifest.architecture.join(",") !== "56,32,1" || run.counters.games !== run.training_games + run.selection_games || run.wall_seconds <= 0 || run.development_seconds <= 0) throw Error("missing model/budget evidence");
const report = fs.readFileSync(`${dir}/experiments/${x.id}/reports/${x.report_sha256}.json`);
if (crypto.createHash("sha256").update(report).digest("hex") !== x.report_sha256) throw Error("research report checksum mismatch");
console.log(JSON.stringify({state:x.state, training_seeds:x.config.training_seeds, seed_runs:x.runs.length, counters:x.counters, final_status:x.verdict.status, measured_seconds:x.wall_seconds, report_sha256:x.report_sha256, artifacts:dir},null,2));
JS
