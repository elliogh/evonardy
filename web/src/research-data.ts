import type { HybridConfig, Job, TDConfig, TrainingConfig } from "./jobs";
export type ResearchConfig = {
  version: string;
  training_seeds: number[];
  methods: {
    algorithm: string;
    ga?: TrainingConfig;
    td?: TDConfig;
    hybrid?: HybridConfig;
  }[];
  development_seed: number;
  final_seed: number;
  development_pairs: number;
  opponent_ids: string[];
  incumbent_id: string;
  workers: number;
  max_turns: number;
  bootstrap: { seed: number; resamples: number; confidence: number };
  confirmation: {
    version: string;
    pairs: number;
    minimum_pairs: number;
    minimum_margin: number;
    bootstrap: ResearchConfig["bootstrap"];
  };
};
export type Estimate = {
  training_seeds: number;
  pairs: number;
  games: number;
  win_rate: number;
  mean_points: number;
  mars_win_rate: number;
  win_interval: { low: number; high: number };
  point_interval: { low: number; high: number };
  degenerate: boolean;
};
export type ResearchRun = {
  algorithm: string;
  seed: number;
  bot_id: string;
  counters: Job["counters"];
  training_games: number;
  selection_games: number;
  wall_seconds: number;
  development_seconds: number;
  development_counters: Job["counters"];
  estimate: Estimate | null;
};
export type Experiment = {
  id: string;
  name: string;
  version: number;
  revision: number;
  state: string;
  phase: string;
  error: string;
  can_resume: boolean;
  config: ResearchConfig;
  counters: Job["counters"];
  wall_seconds: number;
  runs: ResearchRun[] | null;
  active: ResearchRun | null;
  methods: { algorithm: string; estimate: Estimate }[] | null;
  differences: { a: string; b: string; estimate: Estimate }[] | null;
  candidate_id: string;
  selection_locked: boolean;
  final_data_role: string;
  final_data_reused_from: string[];
  final_counters: Job["counters"];
  final_seconds: number;
  verdict: { status: string; reason: string; estimate: Estimate } | null;
  report_sha256: string;
  execution: {
    platform: string;
    go_version: string;
    source_revision: string;
    source_modified: boolean;
    ruleset: string;
    encoder: string;
    network: string;
    schedule: string;
    td_contract: string;
    population_contract: string;
    workers: number;
  };
};
export const methodName = (name: string) =>
  ({
    "ga-mlp": "GA-MLP",
    td0: "TD(0)",
    "td-lambda": "TD(λ)",
    hybrid: "Hybrid",
  })[name] ?? name;
export const researchActive = (x: Experiment) =>
  ["queued", "running", "stopping"].includes(x.state);
export function smokeConfig(defaults: ResearchConfig): ResearchConfig {
  const c = structuredClone(defaults);
  c.training_seeds = [11, 12];
  c.development_pairs = 1;
  c.opponent_ids = ["builtin/heuristic-v1"];
  c.confirmation.pairs = 2;
  c.bootstrap.resamples = 100;
  c.confirmation.bootstrap.resamples = 100;
  for (const m of c.methods) {
    if (m.ga) {
      m.ga.population = 4;
      m.ga.generations = 1;
      m.ga.pairs_per_opponent = 1;
      m.ga.tournament_size = 2;
    }
    if (m.td) m.td.games = 1;
    if (m.hybrid) {
      m.hybrid.rounds = 1;
      m.hybrid.games_per_round = 1;
    }
  }
  return c;
}
