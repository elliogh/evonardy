import { useEffect, useState } from "react";
import { api, command } from "./api";
import type { Bot } from "./api";
import { active, defaults, failure, jobPath, useJob } from "./jobs";
import type {
  Candidate,
  EvaluationConfig,
  Generation,
  Job,
  Metric,
  TrainingConfig,
} from "./jobs";

function NumberField({
  label,
  value,
  onChange,
  min,
  max,
  step = 1,
}: {
  label: string;
  value: number;
  onChange: (n: number) => void;
  min: number;
  max: number;
  step?: number;
}) {
  return (
    <label>
      {label}
      <input
        type="number"
        required
        min={min}
        max={max}
        step={step}
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
      />
    </label>
  );
}
export function Jobs({
  kind,
  id = "",
  initialBot = "",
}: {
  kind: "training" | "evaluation";
  id?: string;
  initialBot?: string;
}) {
  const [runs, setRuns] = useState<Job[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let alive = true;
    api<Job[]>(jobPath(kind))
      .then((x) => {
        if (alive) setRuns(x);
      })
      .catch((e) => {
        if (alive) setError(failure(e));
      });
    return () => {
      alive = false;
    };
  }, [kind, id]);
  if (id) return <JobDetail kind={kind} id={id} />;
  return (
    <>
      <section className="hero lab-hero">
        <p className="eyebrow">
          {kind === "training" ? "TEACH A STRATEGY" : "TEST A FROZEN STRATEGY"}
        </p>
        <h1>
          {kind === "training" ? "Let your bot evolve." : "Put it to the test."}
        </h1>
        <p className="intro">
          {kind === "training"
            ? "Evolve nine weights through real games. Keep any evaluated candidate and make it your own."
            : "Paired games on fresh dice seeds. The selected model stays fixed throughout the evaluation."}
        </p>
      </section>
      {error && (
        <p className="alert" role="alert">
          {error}
        </p>
      )}
      {kind === "training" ? (
        <TrainingForm />
      ) : (
        <EvaluationForm initialBot={initialBot} />
      )}
      <section>
        <div className="section-heading">
          <h2>
            {kind === "training" ? "Training runs" : "Independent evaluations"}
          </h2>
          <span className="muted">Saved locally</span>
        </div>
        {!runs ? (
          <p role="status">Loading runs…</p>
        ) : !runs.length ? (
          <p className="empty-state">
            Your first run will appear here. Every completed batch is
            checkpointed.
          </p>
        ) : (
          <div className="recent-games">
            {runs.map((x) => (
              <a
                className="recent-game"
                key={x.id}
                href={`#${kind === "training" ? "/training" : "/evaluations"}/${x.id}`}
              >
                <span>
                  <strong>{x.name}</strong>
                  <small>
                    {new Date(x.created_at).toLocaleString("en")} ·{" "}
                    {x.counters.games} games ·{" "}
                    {kind === "training"
                      ? `${x.generation} generations`
                      : `${x.evaluation?.stats?.wins ?? "—"} wins`}
                  </small>
                </span>
                <span>{x.state} →</span>
              </a>
            ))}
          </div>
        )}
      </section>
    </>
  );
}
function TrainingForm() {
  const [cfg, setConfig] = useState<TrainingConfig>({ ...defaults });
  const [name, setName] = useState("My first GA bot");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const field = (
    key: keyof TrainingConfig,
    label: string,
    min: number,
    max: number,
    step = 1,
  ) => (
    <NumberField
      label={label}
      value={cfg[key]}
      min={min}
      max={max}
      step={step}
      onChange={(n) => setConfig((c) => ({ ...c, [key]: n }))}
    />
  );
  return (
    <form
      className="lab-panel"
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        setError("");
        try {
          const x = await api<Job>("/training/runs", "POST", {
            ...command(0),
            name,
            config: cfg,
          });
          window.location.hash = `/training/${x.id}`;
        } catch (e) {
          setError(failure(e));
        } finally {
          setBusy(false);
        }
      }}
    >
      <div className="section-heading">
        <div>
          <p className="eyebrow">NEW EXPERIMENT</p>
          <h2>GA-linear</h2>
        </div>
        <button
          type="button"
          disabled={busy}
          onClick={() =>
            setConfig({
              ...defaults,
              population: 4,
              generations: 2,
              pairs_per_opponent: 1,
            })
          }
        >
          Use smoke preset
        </button>
      </div>
      <fieldset disabled={busy} className="form-fields">
        <label className="wide">
          Run name
          <input
            maxLength={128}
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </label>
        {field("seed", "Seed", 0, Number.MAX_SAFE_INTEGER)}
        {field("population", "Population", 4, 128)}
        {field("generations", "Generations", 1, 500)}
        {field("pairs_per_opponent", "Pairs per opponent", 1, 32)}
        {field("workers", "Workers", 1, 8)}
      </fieldset>
      <details>
        <summary>Advanced settings</summary>
        <fieldset disabled={busy} className="form-fields">
          {field("elite_fraction", "Elite fraction", 0.05, 0.5, 0.01)}
          {field("tournament_size", "Tournament size", 2, cfg.population)}
          {field("initial_sigma", "Initial spread", 0.01, 3, 0.01)}
          {field("mutation_sigma", "Mutation scale", 0.01, 3, 0.01)}
          {field("max_turns", "Turn limit per game", 1, 10000)}
        </fieldset>
      </details>
      <p className="muted">
        {(
          cfg.population *
          cfg.generations *
          cfg.pairs_per_opponent *
          4
        ).toLocaleString("en")}{" "}
        games planned against Heuristic and Random, balanced across both colors.
        A truncated game fails the run.
      </p>
      {error && (
        <p className="alert" role="alert">
          {error}
        </p>
      )}
      <button className="primary" disabled={busy}>
        {busy ? "Starting…" : "Start training"}
      </button>
    </form>
  );
}
function EvaluationForm({ initialBot }: { initialBot: string }) {
  const [bots, setBots] = useState<Bot[]>([]);
  const [bot, setBot] = useState(initialBot);
  const [cfg, setConfig] = useState<EvaluationConfig>({
    seed: 2026,
    pairs: 10,
    workers: 2,
    max_turns: 1200,
    opponent_ids: ["builtin/heuristic-v1", "builtin/random-v1"],
  });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let alive = true;
    api<Bot[]>("/bots")
      .then((x) => {
        if (alive) {
          setBots(x.filter((b) => b.available));
          setBot((old) => old || x.find((b) => b.available)?.id || "");
        }
      })
      .catch((e) => {
        if (alive) setError(failure(e));
      });
    return () => {
      alive = false;
    };
  }, []);
  const field = (
    key: "seed" | "pairs" | "workers" | "max_turns",
    label: string,
    min: number,
    max: number,
  ) => (
    <NumberField
      label={label}
      value={cfg[key]}
      min={min}
      max={max}
      onChange={(n) => setConfig((c) => ({ ...c, [key]: n }))}
    />
  );
  return (
    <form
      className="lab-panel"
      onSubmit={async (e) => {
        e.preventDefault();
        setBusy(true);
        setError("");
        try {
          const x = await api<Job>("/evaluations", "POST", {
            ...command(0),
            bot_id: bot,
            config: cfg,
          });
          window.location.hash = `/evaluations/${x.id}`;
        } catch (e) {
          setError(failure(e));
        } finally {
          setBusy(false);
        }
      }}
    >
      <h2>Independent match batch</h2>
      <fieldset className="form-fields" disabled={busy}>
        <label className="wide">
          Model
          <select value={bot} onChange={(e) => setBot(e.target.value)}>
            {bots.map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
        </label>
        {field("seed", "Evaluation seed", 0, Number.MAX_SAFE_INTEGER)}
        {field("pairs", "Pairs per opponent", 1, 500)}
        {field("workers", "Workers", 1, 8)}
        {field("max_turns", "Turn limit per game", 1, 10000)}
      </fieldset>
      <fieldset disabled={busy} className="opponents">
        <legend>Opponents</legend>
        {bots.map((b) => (
          <label key={b.id}>
            <input
              type="checkbox"
              checked={cfg.opponent_ids.includes(b.id)}
              onChange={(e) =>
                setConfig((c) => ({
                  ...c,
                  opponent_ids: e.target.checked
                    ? [...c.opponent_ids, b.id]
                    : c.opponent_ids.filter((id) => id !== b.id),
                }))
              }
            />
            {b.name}
          </label>
        ))}
      </fieldset>
      <p className="muted">
        {cfg.pairs * cfg.opponent_ids.length * 2} games. Every pair swaps colors
        using the same dice schedule. These results are separate from training
        fitness.
      </p>
      {error && (
        <p className="alert" role="alert">
          {error}
        </p>
      )}
      <button
        className="primary"
        disabled={busy || !bot || !cfg.opponent_ids.length}
      >
        {busy ? "Starting…" : "Start evaluation"}
      </button>
    </form>
  );
}
export function FitnessChart({ history }: { history: Metric[] }) {
  if (!history.length)
    return (
      <p className="empty-state">
        Fitness appears after a complete generation. Partial games do not
        receive a score.
      </p>
    );
  const width = 640,
    height = 210;
  const point = (i: number, value: number) =>
    `${45 + (i * 560) / Math.max(1, history.length - 1)},${20 + (1 - value) * 75}`;
  return (
    <>
      <svg
        className="fitness-chart"
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="Saved generation fitness: best in gold, mean in green"
      >
        {[-1, 0, 1].map((n) => (
          <g key={n}>
            <line
              x1={45}
              x2={605}
              y1={20 + (1 - n) * 75}
              y2={20 + (1 - n) * 75}
              className="chart-grid"
            />
            <text x={10} y={25 + (1 - n) * 75}>
              {n}
            </text>
          </g>
        ))}
        <polyline
          className="chart-mean"
          points={history.map((m, i) => point(i, m.mean_fitness)).join(" ")}
        />
        <polyline
          className="chart-best"
          points={history.map((m, i) => point(i, m.best_fitness)).join(" ")}
        />
        {history.map((m, i) => (
          <circle
            key={m.generation}
            cx={point(i, m.best_fitness).split(",")[0]}
            cy={point(i, m.best_fitness).split(",")[1]}
            r={3}
            className="chart-dot"
          >
            <title>{`Generation ${m.generation}: best ${m.best_fitness.toFixed(3)}, mean ${m.mean_fitness.toFixed(3)}`}</title>
          </circle>
        ))}
        <text x={45} y={198}>
          Generation 1
        </text>
        <text x={605} y={198} textAnchor="end">
          {history.length > 1 ? `Generation ${history.length}` : ""}
        </text>
      </svg>
      <p className="muted">
        <span className="gold">● Best</span> · ● Population mean · Fitness =
        wins minus losses / completed games
      </p>
    </>
  );
}
function JobDetail({
  kind,
  id,
}: {
  kind: "training" | "evaluation";
  id: string;
}) {
  const path = `${jobPath(kind)}/${id}`;
  const { x, error, setError, accept, refresh } = useJob(path);
  const [busy, setBusy] = useState(false);
  async function control(op: string) {
    if (!x) return;
    setBusy(true);
    setError("");
    try {
      accept(await api<Job>(`${path}/${op}`, "POST", command(x.version)));
    } catch (e) {
      setError(failure(e));
      await refresh().catch(() => {});
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <a
        className="back-link"
        href={kind === "training" ? "#/training" : "#/evaluations"}
      >
        ← {kind === "training" ? "Training runs" : "Evaluations"}
      </a>
      {error && (
        <p className="alert" role="alert">
          {error}
          <button
            onClick={() =>
              refresh()
                .then(() => setError(""))
                .catch((e) => setError(failure(e)))
            }
          >
            Refresh run
          </button>
        </p>
      )}
      {!x ? (
        <p role="status">Loading run…</p>
      ) : (
        <>
          <div className="game-heading">
            <div>
              <p className="eyebrow">
                {kind === "training"
                  ? "GA-LINEAR"
                  : "INDEPENDENT PAIRED MATCHES"}
              </p>
              <h1>{x.name}</h1>
            </div>
            <span
              className="tag job-state"
              data-state={x.state}
              data-revision={x.revision}
            >
              {x.state}
            </span>
          </div>
          <div className="run-toolbar">
            <p className="muted">
              {x.state === "stopping"
                ? "Finishing the current batch before saving the checkpoint…"
                : x.state === "queued"
                  ? "Waiting for the active job to finish. One job runs at a time."
                  : x.state === "completed"
                    ? "Completed. Results and checkpoint saved."
                    : "Each completed batch is saved locally."}
            </p>
            <div className="actions">
              {active(x) && (
                <button
                  disabled={busy || x.state === "stopping"}
                  onClick={() => control("stop")}
                >
                  Stop and checkpoint
                </button>
              )}
              {x.can_resume && (
                <button
                  className="primary"
                  disabled={busy}
                  onClick={() => control("resume")}
                >
                  Resume run
                </button>
              )}
            </div>
          </div>
          {x.error && (
            <p
              className={x.state === "failed" ? "alert" : "notice"}
              role="status"
            >
              {x.error}
            </p>
          )}
          <div className="run-stats">
            <div>
              <small>Games</small>
              <strong>{x.counters.games.toLocaleString("en")}</strong>
              <span>
                {x.counters.completed_games} completed ·{" "}
                {x.counters.truncated_games} truncated
              </span>
            </div>
            {kind === "training" && (
              <div>
                <small>Generations</small>
                <strong>
                  {x.generation} / {x.config?.generations}
                </strong>
                <span>
                  {x.state === "completed"
                    ? "All generations evaluated"
                    : `${x.generation_games} / ${x.generation_budget} games in current generation`}
                </span>
              </div>
            )}
            <div>
              <small>Decisions</small>
              <strong>{x.counters.decisions.toLocaleString("en")}</strong>
              <span>
                {x.counters.forward_evaluations.toLocaleString("en")} linear
                evaluations
              </span>
            </div>
            <div>
              <small>Active wall time</small>
              <strong>{x.wall_seconds.toFixed(1)}s</strong>
              <span>
                {kind === "training"
                  ? `${x.counters.mutations} mutations · ${x.counters.crossovers} crossovers`
                  : `${x.generation_games} / ${x.generation_budget} games`}
              </span>
            </div>
          </div>
          <progress
            aria-label="Run progress"
            value={kind === "training" ? x.counters.games : x.generation_games}
            max={
              kind === "training"
                ? x.config!.generations * x.generation_budget
                : x.generation_budget
            }
          />
          {kind === "training" ? (
            <>
              <section className="lab-panel">
                <div className="section-heading">
                  <h2>Generation fitness</h2>
                  <span className="tag">DEVELOPMENT GAMES</span>
                </div>
                <FitnessChart history={x.history} />
              </section>
              <Candidates x={x} refresh={refresh} />
            </>
          ) : (
            <EvaluationResult x={x} />
          )}
          <details className="lab-panel">
            <summary>Run settings</summary>
            <pre>
              {JSON.stringify(x.config ?? x.evaluation?.config, null, 2)}
            </pre>
            <p className="muted">Run ID: {x.id}</p>
          </details>
        </>
      )}
    </>
  );
}
function Candidates({ x, refresh }: { x: Job; refresh: () => Promise<void> }) {
  const [generation, setGeneration] = useState(0);
  const [archive, setArchive] = useState<Generation | null>(null);
  const [selected, setSelected] = useState("");
  const [name, setName] = useState("My evolved bot");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<Bot | null>(null);
  const [busy, setBusy] = useState(false);
  const candidates = generation === 0 ? x.candidates : (archive?.ranked ?? []);
  const gen = generation || x.generation;
  const candidate: Candidate | undefined =
    candidates.find((c) => c.id === selected) ?? candidates[0];
  useEffect(() => {
    let alive = true;
    setArchive(null);
    if (generation)
      api<Generation>(`/training/runs/${x.id}/generations/${generation}`)
        .then((g) => {
          if (alive) setArchive(g);
        })
        .catch((e) => {
          if (alive) setError(failure(e));
        });
    return () => {
      alive = false;
    };
  }, [x.id, generation]);
  return (
    <section className="lab-panel">
      <div className="section-heading">
        <div>
          <p className="eyebrow">KEEP A STRATEGY</p>
          <h2>Evaluated candidates</h2>
        </div>
      </div>
      {!x.generation ? (
        <p className="muted">
          Complete a generation to save a candidate. Training can continue after
          you save.
        </p>
      ) : (
        <>
          <div className="form-fields">
            <label>
              Generation
              <select
                aria-label="Candidate generation"
                value={generation}
                disabled={busy}
                onChange={(e) => {
                  setGeneration(Number(e.target.value));
                  setSelected("");
                }}
              >
                <option value={0}>Latest ({x.generation})</option>
                {x.history.map((h) => (
                  <option key={h.generation} value={h.generation}>
                    Generation {h.generation}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Candidate
              <select
                aria-label="Candidate"
                value={candidate?.id ?? ""}
                disabled={busy || !candidates.length}
                onChange={(e) => setSelected(e.target.value)}
              >
                {candidates.map((c, i) => (
                  <option key={c.id} value={c.id}>
                    #{i + 1} · {c.id} · fitness {c.stats.fitness.toFixed(3)}
                  </option>
                ))}
              </select>
            </label>
          </div>
          {candidate && (
            <>
              <p className="muted">
                {candidate.stats.wins} wins / {candidate.stats.games} games ·{" "}
                {candidate.stats.mars_wins} mars wins ·{" "}
                {(candidate.stats.points / candidate.stats.games).toFixed(2)}{" "}
                mean points in development games. Save this candidate to run an
                independent evaluation.
              </p>
              <details>
                <summary>Nine frozen weights</summary>
                <div className="weights">
                  {candidate.weights.map((w, i) => (
                    <span key={i}>
                      {
                        [
                          "Pip",
                          "Head",
                          "Home",
                          "Off",
                          "Occupied",
                          "Block",
                          "Distribution",
                          "Rear",
                          "Mobility",
                        ][i]
                      }
                      <strong>{w.toFixed(4)}</strong>
                    </span>
                  ))}
                </div>
              </details>
              <form
                className="name-form"
                onSubmit={async (e) => {
                  e.preventDefault();
                  setBusy(true);
                  setError("");
                  setNotice(null);
                  try {
                    const bot = await api<Bot>(
                      `/training/runs/${x.id}/save-bot`,
                      "POST",
                      {
                        ...command(x.version),
                        generation: gen,
                        candidate_id: candidate.id,
                        name,
                      },
                    );
                    setNotice(bot);
                    await refresh();
                  } catch (e) {
                    setError(failure(e));
                    await refresh().catch(() => {});
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                <label>
                  Bot name
                  <input
                    maxLength={128}
                    required
                    value={name}
                    disabled={busy}
                    onChange={(e) => setName(e.target.value)}
                  />
                </label>
                <button className="primary" disabled={busy}>
                  Save candidate
                </button>
              </form>
            </>
          )}
        </>
      )}
      {error && (
        <p className="alert" role="alert">
          {error}
        </p>
      )}
      {notice && (
        <p className="notice" role="status">
          Saved as {notice.name}. Identical strategies share one snapshot.{" "}
          <a className="button" href="#">
            Open My bots
          </a>{" "}
          <a className="button" href={`#/evaluations?bot=${notice.id}`}>
            Evaluate this bot
          </a>
        </p>
      )}
    </section>
  );
}
function EvaluationResult({ x }: { x: Job }) {
  const stats = x.evaluation?.stats;
  return (
    <section className="lab-panel">
      <h2>Independent result</h2>
      {!stats ? (
        <p className="empty-state">
          Results appear after every game completes. Stopped or truncated
          batches do not receive an aggregate score.
        </p>
      ) : (
        <>
          <div className="run-stats">
            <div>
              <small>Win rate</small>
              <strong>{((100 * stats.wins) / stats.games).toFixed(1)}%</strong>
              <span>
                {stats.wins} wins / {stats.games} games
              </span>
            </div>
            <div>
              <small>Mean points</small>
              <strong>{(stats.points / stats.games).toFixed(2)}</strong>
              <span>±1 per win/loss, ±2 for mars</span>
            </div>
            <div>
              <small>Mars wins</small>
              <strong>{stats.mars_wins}</strong>
              <span>Completed games only</span>
            </div>
          </div>
          <p className="muted">
            A small batch describes these games; it does not establish playing
            strength. Both colors use paired dice seeds independent of the
            development schedule.
          </p>
        </>
      )}
      <p className="muted">
        Model: {x.evaluation?.bot_id}
        <br />
        Opponents: {x.evaluation?.config.opponent_ids.join(", ")}
      </p>
    </section>
  );
}
