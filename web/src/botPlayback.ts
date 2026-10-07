import type { Player, Position, Snapshot, Step } from "./api";

export const botStepDelay = 1000;

export type BotFrame = {
  position: Position;
  actor: Player;
  dice: number[];
  step: Step | null;
  stage: "roll" | "source" | "destination";
  stepIndex: number;
  totalSteps: number;
  historyIndex: number;
};

const copyPosition = (position: Position): Position => ({
  ...position,
  checkers: position.checkers.map((points) => [...points]),
  borne_off: [...position.borne_off],
  first_done: [...position.first_done],
});

// Replay only steps already committed by Go. This never computes legal moves
// or changes the saved game; the authoritative snapshot ends every playback.
export function botFrames(
  previous: Snapshot | null,
  next: Snapshot,
): BotFrame[] {
  const freshOpening =
    !previous && next.version === 1 && next.history.length === 1;
  if (!previous && !freshOpening) return [];
  const start = previous?.history.length ?? 0;
  if (start >= next.history.length) return [];
  const events = next.history.slice(start);
  const position = copyPosition(next.position);
  // Recover the board before these events from their authoritative final board.
  for (const event of [...events].reverse()) {
    for (const step of [...event.turn.steps].reverse()) {
      if (step.to === 24) position.borne_off[event.actor]--;
      else position.checkers[event.actor][step.to]--;
      position.checkers[event.actor][step.from]++;
    }
  }
  const frames: BotFrame[] = [];
  events.forEach((event, index) => {
    position.turn = event.actor;
    const pushFrame = (
      step: Step | null,
      stepIndex: number,
      stage: BotFrame["stage"],
    ) => {
      if (event.actor === next.human) return;
      frames.push({
        position: copyPosition(position),
        actor: event.actor,
        dice: [...event.dice],
        step,
        stepIndex,
        stage,
        totalSteps: event.turn.steps.length,
        historyIndex: start + index,
      });
    };
    pushFrame(null, 0, "roll");
    event.turn.steps.forEach((step, stepIndex) => {
      pushFrame(step, stepIndex + 1, "source");
      position.checkers[event.actor][step.from]--;
      if (step.to === 24) position.borne_off[event.actor]++;
      else position.checkers[event.actor][step.to]++;
      pushFrame(step, stepIndex + 1, "destination");
    });
  });
  return frames;
}
