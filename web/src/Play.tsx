import { useEffect, useState } from "react";
import { api, color } from "./api";
import type { Bot, GameSummary, Player, Position, Snapshot } from "./api";
import { Board } from "./Board";
import { preferredColor, recalled, remember, startGame } from "./preferences";

// Display-only opening arrangement. The server creates every playable position.
const opening: Position = {
  ruleset: "long-nardy-fnr2026-nocube-v1",
  checkers: [
    Array.from({ length: 24 }, (_, i) => (i === 0 ? 15 : 0)),
    Array.from({ length: 24 }, (_, i) => (i === 12 ? 15 : 0)),
  ],
  borne_off: [0, 0],
  turn: 0,
  starter: 0,
  first_done: [false, false],
};
const message = (error: unknown) =>
  error instanceof Error ? error.message : "something went wrong";

export function Play({ newGame }: { newGame: boolean }) {
  const [bots, setBots] = useState<Bot[] | null>(null);
  const [selected, setSelected] = useState(() => recalled("bot"));
  const [games, setGames] = useState<GameSummary[]>([]);
  const [resume, setResume] = useState<Snapshot | null>(null);
  const [loadingGames, setLoadingGames] = useState(true);
  const [botError, setBotError] = useState("");
  const [gameError, setGameError] = useState("");
  const [error, setError] = useState("");
  const [attempt, setAttempt] = useState(0);
  const [busy, setBusy] = useState(false);
  const [choosing, setChoosing] = useState(false);
  const human = preferredColor();
  useEffect(() => {
    let alive = true;
    setBotError("");
    setGameError("");
    setLoadingGames(true);
    // History and evaluations are not prerequisites for starting a game.
    api<Bot[]>("/bots")
      .then((items) => {
        if (alive) setBots(items);
      })
      .catch((e) => {
        if (alive) setBotError(message(e));
      });
    api<GameSummary[]>("/games")
      .then(async (items) => {
        if (!alive) return;
        const sorted = [...items].sort((a, b) =>
          b.updated_at.localeCompare(a.updated_at),
        );
        setGames(sorted);
        const unfinished = sorted.filter((game) => game.phase !== "finished");
        const current =
          unfinished.find((game) => game.id === recalled("game")) ??
          unfinished[0];
        if (current) {
          const snapshot = await api<Snapshot>(`/games/${current.id}`);
          if (alive) setResume(snapshot.phase === "finished" ? null : snapshot);
        } else setResume(null);
      })
      .catch((e) => {
        if (alive) setGameError(message(e));
      })
      .finally(() => {
        if (alive) setLoadingGames(false);
      });
    return () => {
      alive = false;
    };
  }, [attempt]);
  const bot =
    bots?.find((item) => item.id === selected && item.available) ??
    bots?.find((item) => item.kind === "heuristic" && item.available) ??
    bots?.find((item) => item.available);
  const continuing = !newGame && resume !== null;
  async function begin() {
    if (!bot || busy) return;
    setBusy(true);
    setError("");
    try {
      await startGame(bot.id, human);
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <div className="play-heading">
        <div>
          <h1>Your place at the board.</h1>
        </div>
        <span className="muted">15 checkers. One way home.</span>
      </div>
      <div className="game-layout lobby">
        <div className="table-surface">
          <div className="table-caption">
            <span>
              {continuing ? `You vs ${resume.bot_name}` : "The board is ready"}
            </span>
            <span>{continuing ? "Saved game" : "New game"}</span>
          </div>
          <Board
            position={continuing ? resume.draft_position : opening}
            next={[]}
            selected={null}
            disabled
            onPoint={() => {}}
          />
          <p className="game-help">
            Both colors move counterclockwise. Bring all your checkers home,
            then bear them off.
          </p>
        </div>
        <aside className="play-panel" aria-label="Start or continue a game">
          <h2>
            {continuing ? "Your game is waiting." : "Ready when you are."}
          </h2>
          {continuing ? (
            <>
              <div className="opponent-summary">
                <span className="muted">Your opponent</span>
                <h3>{resume.bot_name}</h3>
                <p className="muted">
                  You play {color(resume.human)} · {resume.history.length} turns
                </p>
              </div>
              <a
                className="button primary start-button"
                href={`#/games/${resume.id}`}
              >
                Continue game <span aria-hidden="true">→</span>
              </a>
              <a className="button start-button" href="#/play/new">
                New game
              </a>
              <p className="panel-note">
                Your dice and unfinished moves are saved.
              </p>
            </>
          ) : (
            <>
              <div className="opponent-summary">
                <div className="opponent-label">
                  <span className="muted">Your opponent</span>
                  {bot && (
                    <button
                      className="text-button"
                      disabled={busy}
                      aria-expanded={choosing}
                      aria-controls="opponent-picker"
                      onClick={() => setChoosing(!choosing)}
                    >
                      {choosing ? "Done" : "Change"}
                    </button>
                  )}
                </div>
                <h3>
                  {bot?.name ??
                    (bots ? "No available bots" : "Loading opponents…")}
                </h3>
                {bot && (
                  <p className="muted">
                    {bot.builtin ? "Built-in opponent" : "Saved strategy"}
                  </p>
                )}
                {choosing && (
                  <div id="opponent-picker">
                    <label htmlFor="opponent">Choose opponent</label>
                    <select
                      id="opponent"
                      value={bot?.id ?? ""}
                      disabled={busy}
                      onChange={(event) => {
                        setSelected(event.target.value);
                        remember("bot", event.target.value);
                      }}
                    >
                      {bots?.map((item) => (
                        <option
                          key={item.id}
                          value={item.id}
                          disabled={!item.available}
                        >
                          {item.name}
                          {!item.available
                            ? ` — unavailable: ${item.reason}`
                            : ""}
                        </option>
                      ))}
                    </select>
                    <a className="inline-link" href="#/bots">
                      Manage your bots
                    </a>
                  </div>
                )}
                {selected &&
                  bots &&
                  !bots.some(
                    (item) => item.id === selected && item.available,
                  ) && (
                    <p className="panel-note">
                      Your previous opponent is unavailable.{" "}
                      {bot
                        ? `${bot.name} is selected instead.`
                        : "Choose an available bot in Training."}
                    </p>
                  )}
              </div>
              <div className="color-preview">
                <span
                  className={`player-stone ${human === 0 ? "white" : "black"}`}
                  aria-hidden="true"
                />
                <span>You play {color(human)}</span>
                <a href="#/settings">Settings</a>
              </div>
              <button
                className="primary start-button"
                disabled={!bot || busy}
                onClick={begin}
              >
                {busy ? "Starting game…" : "Start game"}
                <span aria-hidden="true">→</span>
              </button>
              <p className="panel-note">The opening roll decides who starts.</p>
              {resume && (
                <a className="inline-link" href={`#/games/${resume.id}`}>
                  Continue saved game vs {resume.bot_name}
                </a>
              )}
              {botError && (
                <p className="alert" role="alert">
                  Could not load opponents: {botError}{" "}
                  <button onClick={() => setAttempt(attempt + 1)}>Retry</button>
                </p>
              )}
              {bots && !bot && !botError && (
                <p className="alert" role="status">
                  No playable opponent is available.{" "}
                  <a className="inline-link" href="#/bots">
                    Open My bots
                  </a>
                </p>
              )}
            </>
          )}
          {error && (
            <p className="alert" role="alert">
              Could not start game: {error} Try again.
            </p>
          )}
          {loadingGames && (
            <p className="panel-note" role="status">
              Looking for saved games…
            </p>
          )}
          {gameError && (
            <p className="alert" role="alert">
              Could not load saved games: {gameError} You can still start a new
              game.{" "}
              <button onClick={() => setAttempt(attempt + 1)}>
                Retry history
              </button>
            </p>
          )}
          <div className="training-entry">
            <p>Build your next opponent.</p>
            <a className="inline-link" href="#/training">
              Open Training <span aria-hidden="true">↗</span>
            </a>
          </div>
        </aside>
      </div>
      {games.length > 0 && (
        <details className="recent-section">
          <summary>
            Recent games <span>{games.length}</span>
          </summary>
          <div className="recent-games">
            {games.map((game) => (
              <a
                className="recent-game"
                key={game.id}
                href={`#/games/${game.id}`}
              >
                <span>
                  <strong>You vs {game.bot_name}</strong>
                  <small>
                    {color(game.human)} ·{" "}
                    {new Date(game.updated_at).toLocaleString("en", {
                      dateStyle: "medium",
                      timeStyle: "short",
                    })}
                  </small>
                </span>
                <span>
                  {game.phase === "finished"
                    ? `${color(game.outcome!.winner)} wins →`
                    : "Continue →"}
                </span>
              </a>
            ))}
          </div>
        </details>
      )}
    </>
  );
}

export function Settings() {
  const [human, setHuman] = useState<Player>(preferredColor);
  return (
    <section className="settings-page" aria-labelledby="settings-heading">
      <h1 id="settings-heading">Settings</h1>
      <div className="setting-row">
        <div>
          <h2>Your checker color</h2>
          <p>Used for new games. Your current game stays as it is.</p>
        </div>
        <select
          aria-label="Your color"
          value={human}
          onChange={(event) => {
            const value = Number(event.target.value) as Player;
            setHuman(value);
            remember("color", String(value));
          }}
        >
          <option value={0}>White</option>
          <option value={1}>Black</option>
        </select>
      </div>
      <p className="muted" role="status">
        New games: {color(human)}. Your preference is remembered in this browser
        when storage is available.
      </p>
      <div className="settings-rules">
        <h2>Long nardy</h2>
        <p>
          Both colors move counterclockwise. No doubling cube or tournament
          clock.
        </p>
        <p>
          Bot parameters and training methods are in{" "}
          <a className="inline-link" href="#/training">
            Training
          </a>
          .
        </p>
      </div>
    </section>
  );
}
