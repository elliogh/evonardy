import { useEffect, useRef, useState } from "react";
import { api, color, stepLabel } from "./api";
import type { Outcome, Position, Step } from "./api";
import { Board } from "./Board";
import { active, failure } from "./jobs";
import type { Job } from "./jobs";

type WatchedGame = NonNullable<Job["watched_game"]> & {
  replay: {
    bots: string[];
    opening: number[][];
    events: { dice: number[]; turn: { steps: Step[] } }[];
    outcome: Outcome | null;
    status: string;
  };
  positions: Position[];
};
type View = { game: WatchedGame; frame: number };
const opponentName = (id: string) =>
  id === "builtin/heuristic-v1"
    ? "Heuristic"
    : id === "builtin/random-v1"
      ? "Random"
      : id;

export function TrainingGame({ job }: { job: Job }) {
  const [latest, setLatest] = useState<WatchedGame | null>(null);
  const [view, setView] = useState<View | null>(null);
  const [playing, setPlaying] = useState(true);
  const [follow, setFollow] = useState(true);
  const [speed, setSpeed] = useState(600);
  const [visible, setVisible] = useState(true);
  const [error, setError] = useState("");
  const wantedKey = useRef("");
  const load = useRef<() => void>(() => {});
  useEffect(() => {
    let alive = true,
      pending = false;
    async function fetchLatest() {
      if (pending || !wantedKey.current) return;
      pending = true;
      const requested = wantedKey.current;
      try {
        const game = await api<WatchedGame>(`/training/runs/${job.id}/watch`);
        if (alive) {
          setLatest(game);
          setView((old) => old ?? { game, frame: 0 });
          setError("");
        }
      } catch (e) {
        if (alive) setError(failure(e));
      } finally {
        pending = false;
        // Coalesce changes while a fetch is in flight; keep only the newest sample.
        if (alive && wantedKey.current !== requested) void fetchLatest();
      }
    }
    load.current = () => {
      void fetchLatest();
    };
    return () => {
      alive = false;
    };
  }, [job.id]);
  useEffect(() => {
    wantedKey.current = job.watched_game?.key ?? "";
    load.current();
  }, [job.watched_game?.key]);
  useEffect(() => {
    const changed = () => setVisible(document.visibilityState === "visible");
    changed();
    document.addEventListener("visibilitychange", changed);
    return () => document.removeEventListener("visibilitychange", changed);
  }, []);
  useEffect(() => {
    if (
      !view ||
      !playing ||
      !visible ||
      view.frame >= view.game.replay.events.length
    )
      return;
    const key = view.game.key;
    const frame = view.frame;
    const timer = window.setTimeout(
      () =>
        setView((old) =>
          old?.game.key === key && old.frame === frame
            ? { ...old, frame: old.frame + 1 }
            : old,
        ),
      speed,
    );
    return () => window.clearTimeout(timer);
  }, [view, playing, visible, speed]);
  useEffect(() => {
    if (!view || !playing || view.frame < view.game.replay.events.length)
      return;
    if (follow && latest && latest.key !== view.game.key)
      setView({ game: latest, frame: 0 });
    else if (!follow || !active(job)) setPlaying(false);
  }, [view, playing, follow, latest, job.state]);
  function jump(frame: number) {
    setPlaying(false);
    setView((old) => old && { ...old, frame });
  }
  const newer = latest && latest.key !== view?.game.key;
  const event =
    view && view.frame > 0 ? view.game.replay.events[view.frame - 1] : null;
  const dice = event?.dice ?? view?.game.replay.events[0]?.dice;
  const position = view?.game.positions[view.frame];
  const atEnd = view && view.frame === view.game.replay.events.length;
  const actor = view
    ? view.game.positions[Math.max(0, view.frame - 1)].turn
    : 0;
  const lastPoint = event?.turn.steps.at(-1)?.to;
  return (
    <section
      className="lab-panel training-games"
      aria-labelledby="training-games-heading"
    >
      <div className="section-heading">
        <div>
          <p className="eyebrow">REAL MATCHES FROM THIS RUN</p>
          <h2 id="training-games-heading">Training games</h2>
        </div>
        <span className="tag">SAMPLED PLAYBACK</span>
      </div>
      <p className="muted">
        Games train at full speed. Watch sampled completed games at your own
        pace; playback does not pause learning.
      </p>
      {error && (
        <p className="alert" role="alert">
          {error}
          <button onClick={() => load.current()}>Retry game preview</button>
        </p>
      )}
      {!job.watched_game ? (
        <p className="empty-state">
          {active(job)
            ? "The first completed batch will appear here."
            : "No game was recorded for this run."}
        </p>
      ) : !view || !position ? (
        <p role="status">Loading a training game…</p>
      ) : (
        <div
          className="watched-match"
          data-game-key={view.game.key}
          data-frame={view.frame}
        >
          <div className="watch-heading">
            <div>
              <strong>
                {view.game.candidate_id} vs{" "}
                {opponentName(view.game.opponent_id)}
              </strong>
              <small>
                Generation {view.game.generation} · Game {view.game.index + 1} ·
                Candidate plays {color(view.game.candidate_side)}
              </small>
            </div>
            <button
              disabled={!newer}
              onClick={() => {
                if (latest) setView({ game: latest, frame: 0 });
              }}
            >
              Show latest game
            </button>
          </div>
          <div className="watch-players">
            <span>
              White:{" "}
              {view.game.candidate_side === 0
                ? view.game.candidate_id
                : opponentName(view.game.opponent_id)}
            </span>
            <span>
              Black:{" "}
              {view.game.candidate_side === 1
                ? view.game.candidate_id
                : opponentName(view.game.opponent_id)}
            </span>
          </div>
          <Board
            position={position}
            next={[]}
            selected={
              lastPoint !== undefined && lastPoint < 24 ? lastPoint : null
            }
            disabled
            onPoint={() => {}}
          />
          <div className="watch-move">
            <div
              className="dice"
              aria-label={dice ? `Dice ${dice.join(" and ")}` : "No dice"}
            >
              {dice?.map((die, i) => (
                <span className="die" key={i}>
                  {"⚀⚁⚂⚃⚄⚅"[die - 1]}
                </span>
              ))}
            </div>
            <div>
              <strong>
                {view.frame
                  ? `${color(actor)} moved`
                  : `${color(position.starter)} starts`}
              </strong>
              <p>
                {event
                  ? event.turn.steps.map(stepLabel).join(" · ") || "Pass"
                  : `Opening contest: ${view.game.replay.opening.map((d) => d.join("–")).join(", ")}`}
              </p>
            </div>
          </div>
          {atEnd && (
            <p className="notice match-result" role="status">
              {view.game.replay.outcome
                ? `${color(view.game.replay.outcome.winner)} wins${view.game.replay.outcome.mars ? " · Mars" : ""} · ${view.game.replay.outcome.points} ${view.game.replay.outcome.points === 1 ? "point" : "points"}`
                : "Turn limit reached · No result"}
            </p>
          )}
          <label className="watch-timeline">
            Turn {view.frame} / {view.game.replay.events.length}
            <input
              aria-label="Training game turn"
              type="range"
              min={0}
              max={view.game.replay.events.length}
              value={view.frame}
              onChange={(e) => jump(Number(e.target.value))}
            />
          </label>
          <div className="watch-controls">
            <div className="actions">
              <button
                disabled={view.frame === 0}
                onClick={() => jump(view.frame - 1)}
              >
                Previous turn
              </button>
              <button
                className="primary"
                onClick={() => {
                  if (!playing && atEnd)
                    setView({
                      game: follow && latest ? latest : view.game,
                      frame: 0,
                    });
                  setPlaying(!playing);
                }}
              >
                {playing ? "Pause playback" : "Play replay"}
              </button>
              <button disabled={!!atEnd} onClick={() => jump(view.frame + 1)}>
                Next turn
              </button>
            </div>
            <label>
              Speed
              <select
                aria-label="Playback speed"
                value={speed}
                onChange={(e) => setSpeed(Number(e.target.value))}
              >
                <option value={1200}>Slow</option>
                <option value={600}>Normal</option>
                <option value={200}>Fast</option>
              </select>
            </label>
            <label className="watch-follow">
              <input
                type="checkbox"
                checked={follow}
                onChange={(e) => setFollow(e.target.checked)}
              />
              Follow training
            </label>
          </div>
          {atEnd && playing && active(job) && !newer && (
            <p className="muted">Waiting for another sampled game…</p>
          )}
        </div>
      )}
    </section>
  );
}
