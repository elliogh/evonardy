import { useCallback, useEffect, useState } from "react";
import { api } from "./api";

export type TrainingConfig = {
  seed: number;
  population: number;
  generations: number;
  pairs_per_opponent: number;
  workers: number;
  max_turns: number;
  elite_fraction: number;
  tournament_size: number;
  initial_sigma: number;
  mutation_sigma: number;
};
export const defaults: TrainingConfig = {
  seed: 42,
  population: 64,
  generations: 20,
  pairs_per_opponent: 2,
  workers: 2,
  max_turns: 1200,
  elite_fraction: 0.1,
  tournament_size: 3,
  initial_sigma: 0.4,
  mutation_sigma: 0.15,
};
export type TDConfig = {
  seed: number;
  games: number;
  max_turns: number;
  alpha: number;
  epsilon: number;
  lambda?: number;
};
export const tdDefaults: Required<TDConfig> = {
  seed: 42,
  games: 32,
  max_turns: 1200,
  alpha: 0.001,
  epsilon: 0.05,
  lambda: 0.7,
};
export type TDMetric = {
  game: number;
  status: string;
  outcome: { winner: 0 | 1; mars: boolean; points: number } | null;
  decisions: number;
  forward_evaluations: number;
  updates: number;
  mean_abs_delta: number;
};
export type Bounds = { min: number; max: number };
export type Hyperparameters = {
  alpha: number;
  epsilon: number;
  lambda: number;
};
export type HybridConfig = Hyperparameters & {
  seed: number;
  rounds: number;
  games_per_round: number;
  pairs_per_opponent: number;
  workers: number;
  max_turns: number;
  alpha_bounds: Bounds;
  epsilon_bounds: Bounds;
  lambda_bounds: Bounds;
};
export const hybridDefaults: HybridConfig = {
  seed: 42,
  rounds: 4,
  games_per_round: 8,
  pairs_per_opponent: 1,
  workers: 2,
  max_turns: 1200,
  alpha: 0.001,
  epsilon: 0.05,
  lambda: 0.7,
  alpha_bounds: { min: 0.00001, max: 0.1 },
  epsilon_bounds: { min: 0, max: 0.3 },
  lambda_bounds: { min: 0, max: 0.95 },
};
export const gaMLPDefaults: TrainingConfig = {
  ...defaults,
  population: 8,
  generations: 4,
  initial_sigma: 1,
  mutation_sigma: 0.02,
};
export type NeuroSummary = Omit<Candidate, "weights"> & {
  reason: string;
  hyperparameters?: Hyperparameters;
};
export type Replacement = {
  child_id: string;
  parent_id: string;
  replaced_id: string;
  reason: string;
  before: Hyperparameters;
  after: Hyperparameters;
};
export type EvaluationConfig = {
  seed: number;
  pairs: number;
  workers: number;
  max_turns: number;
  opponent_ids: string[];
};
export type Stats = {
  games: number;
  wins: number;
  mars_wins: number;
  points: number;
  fitness: number;
};
export type Candidate = {
  id: string;
  weights: number[];
  parents: string[];
  stats: Stats;
};
export type Metric = {
  generation: number;
  games: number;
  best_fitness: number;
  mean_fitness: number;
  best_candidate_id: string;
};
export type Generation = {
  neural_ranked?: NeuroSummary[];
  replacements?: Replacement[];
  number: number;
  ranked: Candidate[];
  metric: Metric;
};
export type Job = {
  source_bot_id?: string;
  source_model_sha256?: string;
  hybrid_config?: HybridConfig;
  neural_candidates?: NeuroSummary[];
  replacements?: Replacement[];
  population_progress?: {
    phase: string;
    training_done?: number[];
    training_games: number;
    selection_games: number;
    total_budget: number;
  };
  id: string;
  kind: "training" | "evaluation";
  name: string;
  version: number;
  revision: number;
  state: string;
  can_resume: boolean;
  error: string;
  created_at: string;
  updated_at: string;
  config: TrainingConfig | null;
  algorithm?: string;
  td_config?: TDConfig;
  td_history?: TDMetric[];
  neural_candidate?: { id: string; game: number };
  generation: number;
  generation_games: number;
  generation_budget: number;
  counters: {
    games: number;
    completed_games: number;
    truncated_games: number;
    decisions: number;
    forward_evaluations: number;
    mutations: number;
    crossovers: number;
    updates?: number;
  };
  history: Metric[];
  candidates: Candidate[];
  wall_seconds: number;
  saved: { bot_id: string; generation: number; candidate_id: string }[];
  evaluation: {
    bot_id: string;
    config: EvaluationConfig;
    stats: Stats | null;
  } | null;
  watched_game?: {
    key: string;
    generation: number;
    index: number;
    candidate_id: string;
    opponent_id: string;
    candidate_side: 0 | 1;
    turns: number;
    status: string;
  };
};
export const failure = (e: unknown) =>
  e instanceof Error ? e.message : "Something went wrong";
export const active = (x: Job) =>
  ["queued", "running", "stopping"].includes(x.state);
export const algorithmName = (x: Job) =>
  x.algorithm === "ga-mlp-v1"
    ? "GA-MLP"
    : x.algorithm === "hybrid-sync-v1"
      ? "Hybrid"
      : x.algorithm === "td-zero-v1"
        ? "TD(0)"
        : x.algorithm === "td-lambda-v1"
          ? "TD(lambda)"
          : "GA-linear";
export const jobPath = (kind: string) =>
  kind === "training" ? "/training/runs" : "/evaluations";
export function useJob(path: string) {
  const [x, setX] = useState<Job | null>(null);
  const [error, setError] = useState("");
  const accept = useCallback(
    (next: Job) =>
      setX((old) => (!old || next.revision >= old.revision ? next : old)),
    [],
  );
  const refresh = useCallback(
    async () => accept(await api<Job>(path)),
    [path, accept],
  );
  useEffect(() => {
    let alive = true;
    const sync = () =>
      api<Job>(path)
        .then((next) => {
          if (alive) accept(next);
        })
        .catch((e) => {
          if (alive) setError(failure(e));
        });
    void sync();
    const stream = new EventSource(`/api${path}/events`);
    ["run.progress", "checkpoint.saved", "run.completed", "run.failed"].forEach(
      (event) => stream.addEventListener(event, sync),
    );
    stream.onopen = sync;
    const visible = () => {
      if (document.visibilityState === "visible") void sync();
    };
    document.addEventListener("visibilitychange", visible);
    return () => {
      alive = false;
      stream.close();
      document.removeEventListener("visibilitychange", visible);
    };
  }, [path, accept]);
  return { x, error, setError, accept, refresh };
}
