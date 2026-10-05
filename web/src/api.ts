export type Player = 0 | 1;
export type Step = { from: number; to: number; die: number };
export type Turn = { steps: Step[] };
export type Position = {
  ruleset: string;
  checkers: number[][];
  borne_off: number[];
  turn: Player;
  starter: Player;
  first_done: boolean[];
};
export type Outcome = { winner: Player; points: number; mars: boolean };
export type Bot = {
  id: string;
  name: string;
  kind: string;
  source: string;
  created_at: string;
  builtin: boolean;
  available: boolean;
  reason: string;
  metadata_version: number;
};
export type GameSummary = {
  id: string;
  version: number;
  bot_id: string;
  bot_name: string;
  human: Player;
  phase: "awaiting_roll" | "moving" | "finished";
  created_at: string;
  updated_at: string;
  outcome: Outcome | null;
};
export type Snapshot = GameSummary & {
  position: Position;
  draft_position: Position;
  dice: number[] | null;
  draft: Step[];
  continuations: { next: Step[]; complete: boolean };
  remaining_dice: number[];
  opening: number[][];
  history: { actor: Player; dice: number[]; turn: Turn; after_hash: string }[];
};
export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const encoded = body === undefined ? undefined : JSON.stringify(body);
  for (let attempt = 0; ; attempt++) {
    try {
      const response = await fetch(`/api${path}`, {
        method,
        signal: AbortSignal.timeout(15_000),
        headers:
          encoded === undefined ? {} : { "Content-Type": "application/json" },
        body: encoded,
      });
      const data = await response.json();
      if (!response.ok)
        throw new APIError(
          response.status,
          data.error?.message ?? "Request failed",
        );
      return data as T;
    } catch (error) {
      // Retain the body and command_id if the response was lost after a commit.
      if (attempt >= 1 || (error instanceof APIError && error.status < 500))
        throw error;
    }
  }
}
export const command = (version: number) => ({
  command_id: crypto.randomUUID(),
  expected_version: version,
});
export const color = (player: Player) => (player === 0 ? "White" : "Black");
export const stepLabel = (step: Step) =>
  `${step.from + 1} → ${step.to === 24 ? "off" : step.to + 1} (${step.die})`;
