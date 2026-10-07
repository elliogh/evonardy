import { useCallback, useEffect, useRef, useState } from "react";
import { APIError, api, color, command, stepLabel } from "./api";
import type { Snapshot, Step } from "./api";
import { Board } from "./Board";
import { remember, startGame } from "./preferences";
import { botFrames, botStepDelay } from "./botPlayback";
import type { BotFrame } from "./botPlayback";
const message = (error: unknown) =>
  error instanceof Error ? error.message : "something went wrong";
export function Game({ id }: { id: string }) {
  const [x, setX] = useState<Snapshot | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [selected, setSelected] = useState<number | null>(null);
  const [choices, setChoices] = useState<Step[]>([]);
  const alive = useRef(true);
  const latest = useRef<Snapshot | null>(null);
  const requestPending = useRef(false);
  const attemptedRoll = useRef<number | null>(null);
  const [frames, setFrames] = useState<BotFrame[]>([]);
  const playback = frames[0] ?? null;
  const playing = playback !== null;
  const accept = useCallback((next: Snapshot) => {
    if (
      !alive.current ||
      (latest.current && next.version <= latest.current.version)
    )
      return;
    const addedFrames = botFrames(latest.current, next);
    latest.current = next;
    setX(next);
    if (addedFrames.length)
      setFrames((pending) => [...pending, ...addedFrames]);
  }, []);
  const refresh = useCallback(
    async () => accept(await api<Snapshot>(`/games/${id}`)),
    [id, accept],
  );
  useEffect(() => {
    alive.current = true;
    remember("game", id);
    refresh().catch((e) => {
      if (alive.current) setError(message(e));
    });
    const stream = new EventSource(`/api/games/${id}/events`);
    const sync = () =>
      refresh().catch((e) => {
        if (alive.current) setError(message(e));
      });
    stream.addEventListener("game.updated", sync);
    stream.onopen = sync;
    const visible = () => {
      if (document.visibilityState === "visible") sync();
    };
    document.addEventListener("visibilitychange", visible);
    return () => {
      alive.current = false;
      stream.close();
      document.removeEventListener("visibilitychange", visible);
    };
  }, [id, refresh]);
  useEffect(() => {
    setSelected(null);
    setChoices([]);
  }, [x?.version]);
  useEffect(() => {
    if (!playback) return;
    const timer = window.setTimeout(
      () => setFrames((pending) => pending.slice(1)),
      botStepDelay,
    );
    return () => window.clearTimeout(timer);
  }, [playback]);
  const mutate = useCallback(
    async (action: string, extra = {}) => {
      if (!x || requestPending.current || playing) return;
      requestPending.current = true;
      setBusy(true);
      setError("");
      const body = { ...command(x.version), ...extra };
      try {
        accept(await api<Snapshot>(`/games/${id}/${action}`, "POST", body));
      } catch (e) {
        // Another tab may already have rolled this turn. Refresh instead of
        // presenting a conflict as a failed roll or assigning new dice again.
        if (
          !(action === "roll" && e instanceof APIError && e.status === 409) &&
          alive.current
        ) {
          setError(message(e));
        }
        await refresh().catch(() => {});
      } finally {
        requestPending.current = false;
        if (alive.current) setBusy(false);
      }
    },
    [x, playing, id, accept, refresh],
  );
  useEffect(() => {
    if (
      !x ||
      x.phase !== "awaiting_roll" ||
      playing ||
      busy ||
      attemptedRoll.current === x.version
    )
      return;
    attemptedRoll.current = x.version;
    void mutate("roll");
  }, [x, playing, busy, mutate]);
  function move(step: Step) {
    if (x) {
      setChoices([]);
      void mutate("continuations", { prefix: [...x.draft, step] });
    }
  }
  function point(point: number) {
    if (!x || playing || busy) return;
    const matches = x.continuations.next.filter(
      (step) => step.from === selected && step.to === point,
    );
    if (matches.length === 1) move(matches[0]);
    else if (matches.length > 1) setChoices(matches);
    else if (x.continuations.next.some((step) => step.from === point)) {
      setSelected(point);
      setChoices([]);
    }
  }
  return (
    <>
      <a className="back-link" href="#/play/new">
        ← New game
      </a>
      {error && (
        <p className="alert" role="alert">
          {error}{" "}
          <button
            disabled={busy || playing}
            onClick={() =>
              x?.phase === "awaiting_roll"
                ? void mutate("roll")
                : void refresh()
                    .then(() => setError(""))
                    .catch((e) => setError(message(e)))
            }
          >
            {x?.phase === "awaiting_roll"
              ? "Retry automatic roll"
              : "Refresh game"}
          </button>
        </p>
      )}
      {!x ? (
        <p role="status">Loading your game…</p>
      ) : (
        <>
          <div className="game-heading">
            <div>
              <h1>You vs {x.bot_name}</h1>
            </div>
            <span className="tag">{x.history.length} TURNS</span>
          </div>
          <div className="game-layout">
            <div>
              <div
                className="turn-status"
                aria-live="polite"
                data-phase={playing ? "bot_playback" : x.phase}
                data-playback={playing ? "playing" : "idle"}
                data-playback-stage={playback?.stage}
                data-version={x.version}
              >
                <strong>
                  {playback
                    ? playback.step
                      ? playback.stage === "source"
                        ? `${x.bot_name}: selects point ${playback.step.from + 1} · die ${playback.step.die}`
                        : `${x.bot_name}: ${stepLabel(playback.step)}`
                      : playback.totalSteps
                        ? `${x.bot_name} rolled ${playback.dice.join(" and ")}.`
                        : `${x.bot_name} has no legal move and passes.`
                    : x.phase === "finished"
                      ? `${color(x.outcome!.winner)} wins${x.outcome!.mars ? " · Mars" : ""}`
                      : x.phase === "awaiting_roll"
                        ? "Rolling your dice automatically…"
                        : x.continuations.complete
                          ? "Your turn is ready to confirm."
                          : selected === null
                            ? "Choose a highlighted checker."
                            : "Choose a highlighted destination."}
                </strong>
                <span>
                  {playback
                    ? `Move ${playback.stepIndex} of ${playback.totalSteps}`
                    : x.phase === "finished"
                      ? `${x.outcome!.points} ${x.outcome!.points === 1 ? "point" : "points"}`
                      : `You are ${color(x.human)}`}
                </span>
              </div>
              <Board
                position={playback?.position ?? x.draft_position}
                next={playing ? [] : x.continuations.next}
                selected={playing ? null : selected}
                highlight={playback?.step ?? undefined}
                highlightStage={
                  playback?.stage === "source" ? "source" : "destination"
                }
                disabled={busy || playing || x.phase !== "moving"}
                onPoint={point}
              />
              {!playing && choices.length > 0 && (
                <fieldset className="die-choice">
                  <legend>Choose which die to use</legend>
                  {choices.map((step) => (
                    <button
                      key={step.die}
                      disabled={busy}
                      onClick={() => move(step)}
                    >
                      Move with {step.die}
                    </button>
                  ))}
                </fieldset>
              )}
              <p className="game-help">
                Both colors move counterclockwise. Select a checker, then its
                destination. Your draft is saved; undo it before confirming.
              </p>
            </div>
            <aside
              className="match-panel"
              aria-label="Game controls and history"
            >
              <div className="opponent-summary">
                <h2>{x.bot_name}</h2>
                <p className="muted">You play {color(x.human)}</p>
              </div>
              <div className="turn-controls">
                <div
                  className="dice"
                  aria-label={
                    playback
                      ? `${x.bot_name} dice ${playback.dice.join(" and ")}`
                      : x.dice
                        ? `Dice ${x.dice.join(" and ")}`
                        : "No dice assigned"
                  }
                >
                  {(playback?.dice ?? x.dice ?? [null, null]).map((d, i) => (
                    <span className="die" key={i}>
                      {d ? "⚀⚁⚂⚃⚄⚅"[d - 1] : "·"}
                    </span>
                  ))}
                  <small>
                    {playback
                      ? playback.step
                        ? `Die ${playback.step.die} · move ${playback.stepIndex} of ${playback.totalSteps}`
                        : "Opponent’s roll"
                      : x.dice
                        ? `Remaining: ${x.remaining_dice.join(", ") || "none"}`
                        : x.phase === "finished"
                          ? "Game complete"
                          : "Rolling automatically…"}
                  </small>
                </div>
                <div className="actions">
                  {playing && (
                    <p className="playback-note" role="status">
                      Watching {x.bot_name}’s turn…
                    </p>
                  )}
                  {!playing && x.phase === "awaiting_roll" && (
                    <p className="playback-note" role="status">
                      {error
                        ? "Automatic roll paused. Retry above."
                        : "Rolling your dice…"}
                    </p>
                  )}
                  {!playing && x.phase === "moving" && (
                    <>
                      <button
                        disabled={busy || x.draft.length === 0}
                        onClick={() =>
                          mutate("continuations", {
                            prefix: x.draft.slice(0, -1),
                          })
                        }
                      >
                        Undo
                      </button>
                      <button
                        disabled={busy || x.draft.length === 0}
                        onClick={() => mutate("continuations", { prefix: [] })}
                      >
                        Reset turn
                      </button>
                      <button
                        className="primary"
                        disabled={busy || !x.continuations.complete}
                        onClick={() =>
                          mutate("turn", { turn: { steps: x.draft } })
                        }
                      >
                        {x.continuations.complete && x.draft.length === 0
                          ? "Pass turn"
                          : "Confirm turn"}
                      </button>
                    </>
                  )}
                  {!playing && x.phase === "finished" && (
                    <>
                      <button
                        className="primary"
                        disabled={busy}
                        onClick={async () => {
                          setBusy(true);
                          setError("");
                          try {
                            await startGame(x.bot_id, x.human);
                          } catch (e) {
                            setError(message(e));
                          } finally {
                            setBusy(false);
                          }
                        }}
                      >
                        Rematch
                      </button>
                      <a className="button" href="#/play/new">
                        Other opponent
                      </a>
                      <a className="replay-link" href={`/api/replays/${x.id}`}>
                        Download replay
                      </a>
                    </>
                  )}
                </div>
              </div>
              <p className="draft-summary">
                {playing
                  ? "The highlighted checker shows each move, before and after. Your dice roll automatically when the opponent finishes."
                  : x.draft.length
                    ? `Your draft: ${x.draft.map(stepLabel).join(" · ")}`
                    : "Moves become final when you confirm the complete turn."}
              </p>
              <div className="history-panel">
                <h2>Turn history</h2>
                <p className="opening">
                  Opening contest:{" "}
                  {x.opening.map((d) => d.join("–")).join(", ")}
                  <br />
                  {color(x.position.starter)} started with the winning dice.
                </p>
                {!x.history.length && (
                  <p className="muted">The first move is yours.</p>
                )}
                <ol className="history-list">
                  {x.history
                    .slice(0, playback?.historyIndex)
                    .map((event, i) => (
                      <li key={i}>
                        <div>
                          <strong>{color(event.actor)}</strong>
                          <span>{event.dice.join("–")}</span>
                        </div>
                        <p>
                          {event.turn.steps.map(stepLabel).join(" · ") ||
                            "Pass"}
                        </p>
                      </li>
                    ))}
                </ol>
              </div>
            </aside>
          </div>
        </>
      )}
    </>
  );
}
