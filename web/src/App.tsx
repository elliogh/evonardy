import { useCallback, useEffect, useRef, useState } from "react";
import { api, color, command, stepLabel } from "./api";
import type { Bot, GameSummary, Player, Snapshot, Step } from "./api";
import { Board } from "./Board";
import { Research } from "./Research";
import { Jobs } from "./Training";
import type { Job } from "./jobs";
const route = () =>
  typeof window === "undefined" ? "" : window.location.hash.slice(1);
const message = (error: unknown) =>
  error instanceof Error ? error.message : "Something went wrong";
export function App() {
  const [path, setID] = useState(route);
  const gameID = path.match(/^\/games\/([A-Za-z0-9_-]+)$/)?.[1];
  const research = path.match(/^\/research(?:\/([A-Za-z0-9_-]+))?$/);
  const training = path.match(/^\/training(?:\/([A-Za-z0-9_-]+))?$/);
  const evaluation = path.match(
    /^\/evaluations(?:\/([A-Za-z0-9_-]+))?(?:\?bot=([^&]+))?$/,
  );
  useEffect(() => {
    const changed = () => setID(route());
    window.addEventListener("hashchange", changed);
    return () => window.removeEventListener("hashchange", changed);
  }, []);
  return (
    <main>
      <header className="site-header">
        <a href="#" className="brand">
          <span className="mark" aria-hidden="true">
            ● ●
          </span>
          EvoNardy
        </a>
        <span className="local-badge">
          <i /> Runs locally
        </span>
      </header>
      <nav className="site-nav" aria-label="Main navigation">
        <a
          href="#"
          aria-current={
            !training && !evaluation && !research ? "page" : undefined
          }
        >
          My bots
        </a>
        <a href="#/training" aria-current={training ? "page" : undefined}>
          Training
        </a>
        <a href="#/evaluations" aria-current={evaluation ? "page" : undefined}>
          Evaluate
        </a>
        <a href="#/research" aria-current={research ? "page" : undefined}>
          Research
        </a>
      </nav>
      {gameID ? (
        <Game key={gameID} id={gameID} />
      ) : research ? (
        <Research key={path} id={research[1]} />
      ) : training ? (
        <Jobs key={path} kind="training" id={training[1]} />
      ) : evaluation ? (
        <Jobs
          key={path}
          kind="evaluation"
          id={evaluation[1]}
          initialBot={evaluation[2]}
        />
      ) : (
        <Library />
      )}
      <footer>
        Long nardy · No doubling cube
        <br />
        <span>Ruleset long-nardy-fnr2026-nocube-v1</span>
      </footer>
    </main>
  );
}
function Library() {
  const [bots, setBots] = useState<Bot[] | null>(null);
  const [games, setGames] = useState<GameSummary[]>([]);
  const [evaluations, setEvaluations] = useState<Job[]>([]);
  const [human, setHuman] = useState<Player>(0);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [saveBot, setSaveBot] = useState<Bot | null>(null);
  const [renameBot, setRenameBot] = useState<Bot | null>(null);
  const [name, setName] = useState("");
  const [notice, setNotice] = useState("");
  const refresh = useCallback(async () => {
    const [b, g, evaluations] = await Promise.all([
      api<Bot[]>("/bots"),
      api<GameSummary[]>("/games"),
      api<Job[]>("/evaluations"),
    ]);
    setBots(b);
    setGames(g);
    setEvaluations(
      evaluations
        .filter((x) => x.state === "completed" && x.evaluation?.stats)
        .sort((a, b) => b.updated_at.localeCompare(a.updated_at)),
    );
  }, []);
  useEffect(() => {
    refresh().catch((e) => setError(message(e)));
  }, [refresh]);
  async function perform(work: () => Promise<void>) {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await work();
    } catch (e) {
      setError(message(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <section className="hero">
        <p className="eyebrow">YOUR LOCAL NARDY LAB</p>
        <h1>
          A good game.
          <br />A bot of your own.
        </h1>
        <p className="intro">
          Play a baseline, evolve your own strategy, and keep a bot worth
          testing.
        </p>
      </section>
      {error && (
        <p className="alert" role="alert">
          {error}{" "}
          <button
            onClick={() =>
              refresh()
                .then(() => setError(""))
                .catch((e) => setError(message(e)))
            }
          >
            Retry
          </button>
        </p>
      )}
      {notice && (
        <p className="notice" role="status">
          {notice}
        </p>
      )}
      <section aria-labelledby="bots-heading">
        <div className="section-heading">
          <div>
            <p className="eyebrow">THE LIBRARY</p>
            <h2 id="bots-heading">My bots</h2>
          </div>
          <label className="color-select">
            Play as{" "}
            <select
              aria-label="Your color"
              value={human}
              disabled={busy}
              onChange={(e) => setHuman(Number(e.target.value) as Player)}
            >
              <option value={0}>White</option>
              <option value={1}>Black</option>
            </select>
          </label>
        </div>
        {!bots && (
          <p className="muted" role="status">
            Loading your library…
          </p>
        )}
        <div className="bot-grid">
          {bots?.map((bot) => (
            <article className="bot-card" key={bot.id}>
              <div className="card-top">
                <span className={`bot-icon ${bot.kind}`} aria-hidden="true">
                  {bot.kind === "random" ? "⚄" : "◈"}
                </span>
                <span className="tag">
                  {bot.builtin ? "BASELINE" : "SAVED SNAPSHOT"}
                </span>
              </div>
              <h3>{bot.name}</h3>
              <p>
                {bot.kind === "random"
                  ? "A fresh choice from every legal position."
                  : bot.builtin
                    ? "A steady eye for distance, home, and blocks."
                    : bot.kind === "tanh-56-32-1-v1"
                      ? "A frozen neural strategy, ready to play."
                      : "A frozen linear strategy, ready to play."}
              </p>
              <dl>
                <div>
                  <dt>Source</dt>
                  <dd>{bot.source}</dd>
                </div>
                <div>
                  <dt>Evaluation</dt>
                  <dd>
                    {(() => {
                      const e = evaluations.find(
                        (x) => x.evaluation?.bot_id === bot.id,
                      );
                      const s = e?.evaluation?.stats;
                      return s ? (
                        <a href={`#/evaluations/${e!.id}`}>
                          {((s.wins / s.games) * 100).toFixed(1)}% wins ·{" "}
                          {s.games} games →
                        </a>
                      ) : (
                        "Not evaluated"
                      );
                    })()}
                  </dd>
                </div>
              </dl>
              {!bot.available && <p className="alert">{bot.reason}</p>}
              <div className="card-actions">
                <a className="button" href={`#/evaluations?bot=${bot.id}`}>
                  Evaluate {bot.name}
                </a>
                <button
                  className="primary"
                  disabled={busy || !bot.available}
                  onClick={() =>
                    perform(async () => {
                      const x = await api<Snapshot>("/games", "POST", {
                        ...command(0),
                        bot_id: bot.id,
                        human,
                      });
                      window.location.hash = `/games/${x.id}`;
                    })
                  }
                >
                  Play against {bot.name}
                </button>
                <button
                  disabled={busy || !bot.available}
                  onClick={() => {
                    setSaveBot(bot);
                    setRenameBot(null);
                    setName(`${bot.name} snapshot`);
                  }}
                >
                  Save snapshot
                </button>
                {!bot.builtin && (
                  <button
                    disabled={busy || !bot.available}
                    onClick={() => {
                      setRenameBot(bot);
                      setSaveBot(null);
                      setName(bot.name);
                    }}
                  >
                    Rename {bot.name}
                  </button>
                )}
              </div>
            </article>
          ))}
        </div>
        {(saveBot || renameBot) && (
          <form
            className="name-form"
            onSubmit={(e) => {
              e.preventDefault();
              perform(async () => {
                if (renameBot)
                  await api(`/bots/${renameBot.id}/metadata`, "PATCH", {
                    ...command(renameBot.metadata_version),
                    name,
                  });
                else if (saveBot) {
                  const card = await api<Bot>("/bots", "POST", {
                    ...command(0),
                    source_bot_id: saveBot.id,
                    name,
                  });
                  setNotice(
                    `Saved as ${card.name}. Identical strategies share one snapshot.`,
                  );
                }
                setSaveBot(null);
                setRenameBot(null);
                await refresh();
              });
            }}
          >
            <label>
              {renameBot ? "Rename snapshot" : "Save a baseline snapshot"}
              <input
                aria-label="Snapshot name"
                maxLength={128}
                required
                value={name}
                onChange={(e) => setName(e.target.value)}
                autoFocus
              />
            </label>
            <button className="primary" disabled={busy || !name.trim()}>
              {renameBot ? "Apply name" : "Save bot"}
            </button>
            <button
              type="button"
              disabled={busy}
              onClick={() => {
                setSaveBot(null);
                setRenameBot(null);
              }}
            >
              Cancel
            </button>
          </form>
        )}
      </section>
      <section aria-labelledby="recent-heading">
        <div className="section-heading">
          <div>
            <p className="eyebrow">PICK UP A GAME</p>
            <h2 id="recent-heading">Recent games</h2>
          </div>
          <span className="muted">{games.length} saved</span>
        </div>
        {games.length === 0 ? (
          <p className="empty-state">
            Your first game starts with a bot above. Dice and moves are saved as
            you play.
          </p>
        ) : (
          <div className="recent-games">
            {games.map((g) => (
              <a className="recent-game" key={g.id} href={`#/games/${g.id}`}>
                <span>
                  <strong>You vs {g.bot_name}</strong>
                  <small>
                    Playing {color(g.human)} ·{" "}
                    {new Date(g.updated_at).toLocaleString("en", {
                      dateStyle: "medium",
                      timeStyle: "short",
                    })}
                  </small>
                </span>
                <span>
                  {g.phase === "finished"
                    ? `${color(g.outcome!.winner)} wins →`
                    : "Resume game →"}
                </span>
              </a>
            ))}
          </div>
        )}
      </section>
      <aside className="roadmap">
        <span className="tag">GA-LINEAR</span>
        <p>
          Train through real games, save an evaluated candidate, then test it on
          fresh dice seeds.
        </p>
        <a className="button" href="#/training">
          Start training
        </a>
      </aside>
    </>
  );
}
function Game({ id }: { id: string }) {
  const [x, setX] = useState<Snapshot | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [selected, setSelected] = useState<number | null>(null);
  const [choices, setChoices] = useState<Step[]>([]);
  const alive = useRef(true);
  const accept = useCallback((next: Snapshot) => {
    if (alive.current)
      setX((old) => (!old || next.version >= old.version ? next : old));
  }, []);
  const refresh = useCallback(
    async () => accept(await api<Snapshot>(`/games/${id}`)),
    [id, accept],
  );
  useEffect(() => {
    alive.current = true;
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
  async function mutate(action: string, extra = {}) {
    if (!x || busy) return;
    setBusy(true);
    setError("");
    const body = { ...command(x.version), ...extra };
    try {
      accept(await api<Snapshot>(`/games/${id}/${action}`, "POST", body));
    } catch (e) {
      setError(message(e));
      await refresh().catch(() => {});
    } finally {
      setBusy(false);
    }
  }
  function move(step: Step) {
    if (x) {
      setChoices([]);
      void mutate("continuations", { prefix: [...x.draft, step] });
    }
  }
  function point(point: number) {
    if (!x) return;
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
      <a className="back-link" href="#">
        ← My bots
      </a>
      {error && (
        <p className="alert" role="alert">
          {error}{" "}
          <button
            onClick={() =>
              refresh()
                .then(() => setError(""))
                .catch((e) => setError(message(e)))
            }
          >
            Refresh game
          </button>
        </p>
      )}
      {!x ? (
        <p role="status">Loading your game…</p>
      ) : (
        <>
          <div className="game-heading">
            <div>
              <p className="eyebrow">
                LONG NARDY · YOU PLAY {color(x.human).toUpperCase()}
              </p>
              <h1>You vs {x.bot_name}</h1>
            </div>
            <span className="tag">{x.history.length} TURNS</span>
          </div>
          <div className="game-layout">
            <div>
              <div
                className="turn-status"
                aria-live="polite"
                data-phase={x.phase}
                data-version={x.version}
              >
                <strong>
                  {x.phase === "finished"
                    ? `${color(x.outcome!.winner)} wins${x.outcome!.mars ? " · Mars" : ""}`
                    : x.phase === "awaiting_roll"
                      ? "Your turn. Roll the dice."
                      : x.continuations.complete
                        ? "Your turn is ready to confirm."
                        : selected === null
                          ? "Choose a highlighted checker."
                          : "Choose a highlighted destination."}
                </strong>
                <span>
                  {x.phase === "finished"
                    ? `${x.outcome!.points} ${x.outcome!.points === 1 ? "point" : "points"}`
                    : `You are ${color(x.human)}`}
                </span>
              </div>
              <Board
                position={x.draft_position}
                next={x.continuations.next}
                selected={selected}
                disabled={busy || x.phase !== "moving"}
                onPoint={point}
              />
              {choices.length > 0 && (
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
              <div className="turn-controls">
                <div
                  className="dice"
                  aria-label={
                    x.dice ? `Dice ${x.dice.join(" and ")}` : "No dice assigned"
                  }
                >
                  {(x.dice ?? [null, null]).map((d, i) => (
                    <span className="die" key={i}>
                      {d ? "⚀⚁⚂⚃⚄⚅"[d - 1] : "·"}
                    </span>
                  ))}
                  <small>
                    {x.dice
                      ? `Remaining: ${x.remaining_dice.join(", ") || "none"}`
                      : "Ready for the next roll"}
                  </small>
                </div>
                <div className="actions">
                  {x.phase === "awaiting_roll" && (
                    <button
                      className="primary"
                      disabled={busy}
                      onClick={() => mutate("roll")}
                    >
                      Roll dice
                    </button>
                  )}
                  {x.phase === "moving" && (
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
                  {x.phase === "finished" && (
                    <a className="button primary" href={`/api/replays/${x.id}`}>
                      Download replay
                    </a>
                  )}
                </div>
              </div>
              <p className="draft-summary">
                {x.draft.length
                  ? `Your draft: ${x.draft.map(stepLabel).join(" · ")}`
                  : "Moves become final when you confirm the complete turn."}
              </p>
              <p className="game-help">
                Both colors move counterclockwise. Select a checker, then its
                destination. Your draft is saved; undo it before confirming.
              </p>
            </div>
            <aside className="history-panel">
              <p className="eyebrow">THE GAME SO FAR</p>
              <h2>Turn history</h2>
              <p className="opening">
                Opening contest: {x.opening.map((d) => d.join("–")).join(", ")}
                <br />
                {color(x.position.starter)} started with the winning dice.
              </p>
              {!x.history.length && (
                <p className="muted">The first move is yours.</p>
              )}
              <ol className="history-list">
                {x.history.map((event, i) => (
                  <li key={i}>
                    <div>
                      <strong>{color(event.actor)}</strong>
                      <span>{event.dice.join("–")}</span>
                    </div>
                    <p>
                      {event.turn.steps.map(stepLabel).join(" · ") || "Pass"}
                    </p>
                  </li>
                ))}
              </ol>
            </aside>
          </div>
        </>
      )}
    </>
  );
}
