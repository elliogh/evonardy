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
  number: number;
  ranked: Candidate[];
  metric: Metric;
};
export type Job = {
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
};
export const failure = (e: unknown) =>
  e instanceof Error ? e.message : "Something went wrong";
export const active = (x: Job) =>
  ["queued", "running", "stopping"].includes(x.state);
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
