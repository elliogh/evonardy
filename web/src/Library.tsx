import { useCallback, useEffect, useState } from "react";
import { api, command } from "./api";
import type { Bot } from "./api";
import type { Job } from "./jobs";
import { startGame } from "./preferences";
const message = (error: unknown) =>
  error instanceof Error ? error.message : "something went wrong";
export function Library() {
  const [bots, setBots] = useState<Bot[] | null>(null);
  const [evaluations, setEvaluations] = useState<Job[]>([]);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [saveBot, setSaveBot] = useState<Bot | null>(null);
  const [renameBot, setRenameBot] = useState<Bot | null>(null);
  const [name, setName] = useState("");
  const [notice, setNotice] = useState("");
  const refresh = useCallback(async () => {
    const [b, evaluations] = await Promise.all([
      api<Bot[]>("/bots"),
      api<Job[]>("/evaluations"),
    ]);
    setBots(b);
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
            <h2 id="bots-heading">My bots</h2>
          </div>
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
                      await startGame(bot.id);
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
    </>
  );
}
