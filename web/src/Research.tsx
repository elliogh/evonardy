import { useEffect, useState } from "react";
import { api, command } from "./api";
import { failure } from "./jobs";
import { methodName, researchActive, smokeConfig } from "./research-data";
import type {
  Estimate,
  Experiment,
  ResearchConfig,
  ResearchRun,
} from "./research-data";
const seedLabel = (seed: number) =>
  Number.isSafeInteger(seed) ? String(seed) : "See report";
const percent = (x: number) => `${(x * 100).toFixed(1)}%`;
const interval = (e: Estimate) =>
  `${percent(e.win_interval.low)} – ${percent(e.win_interval.high)}`;
const colors = ["#88ad86", "#e2a574", "#b7b8ed", "#dca2c2"];

export function Research({ id = "" }: { id?: string }) {
  const [runs, setRuns] = useState<Experiment[] | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let alive = true;
    api<Experiment[]>("/experiments")
      .then((x) => {
        if (alive) setRuns(x);
      })
      .catch((e) => {
        if (alive) setError(failure(e));
      });
    return () => {
      alive = false;
    };
  }, [id]);
  if (id) return <ResearchDetail key={id} id={id} />;
  return (
    <>
      <section className="hero lab-hero">
        <p className="eyebrow">REPRODUCIBLE RESEARCH</p>
        <h1>Compare the evidence.</h1>
        <p className="intro">
          Train multiple independent seeds, compare frozen models on paired
          games, then confirm one candidate on held-out dice.
        </p>
      </section>
      {error && (
        <p className="alert" role="alert">
          {error}
        </p>
      )}
      <ResearchForm />
      <section>
        <div className="section-heading">
          <h2>Experiments</h2>
          <span className="muted">Saved locally</span>
        </div>
        {runs === null ? (
          <p role="status">Loading experiments…</p>
        ) : runs.length === 0 ? (
          <p className="empty-state">Your first experiment will appear here.</p>
        ) : (
          <div className="recent-games">
            {runs.map((x) => (
              <a className="recent-game" key={x.id} href={`#/research/${x.id}`}>
                <span>
                  <strong>{x.name}</strong>
                  <small>
                    {x.config.training_seeds.length} seeds · {x.counters.games}{" "}
                    physical games · {x.phase}
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
function ResearchForm() {
  const [defaults, setDefaults] = useState<ResearchConfig | null>(null);
  const [text, setText] = useState("");
  const [name, setName] = useState("Research experiment");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    let alive = true;
    api<ResearchConfig>("/experiments/config")
      .then((x) => {
        if (alive) {
          setDefaults(x);
          setText(JSON.stringify(x, null, 2));
        }
      })
      .catch((e) => {
        if (alive) setError(failure(e));
      });
    return () => {
      alive = false;
    };
  }, []);
  async function start(e: React.FormEvent) {
    e.preventDefault();
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const config: ResearchConfig = JSON.parse(text);
      if (
        !config ||
        !Array.isArray(config.training_seeds) ||
        !config.bootstrap ||
        !config.confirmation?.bootstrap
      )
        throw new Error(
          "Use a full configuration with training seeds and development/final bootstrap settings.",
        );
      if (
        !config.training_seeds.every(Number.isSafeInteger) ||
        ![
          config.development_seed,
          config.final_seed,
          config.bootstrap.seed,
          config.confirmation.bootstrap.seed,
        ].every(Number.isSafeInteger)
      )
        throw new Error(
          "Use integer seeds within JavaScript's exact numeric range.",
        );
      const x = await api<Experiment>("/experiments", "POST", {
        ...command(0),
        name,
        config,
      });
      location.hash = `/research/${x.id}`;
    } catch (e) {
      setError(failure(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <form className="lab-panel lab-form research-form" onSubmit={start}>
      <div className="section-heading">
        <h2>Experiment configuration</h2>
        <button
          type="button"
          disabled={!defaults || busy}
          onClick={() => {
            if (defaults)
              setText(JSON.stringify(smokeConfig(defaults), null, 2));
          }}
        >
          Use research smoke preset
        </button>
      </div>
      <p>
        Default: five training seeds, GA-MLP, TD(λ) and Hybrid. Every seed
        contributes to the comparison. The smoke preset checks execution and
        cannot confirm a champion.
      </p>
      <label>
        Experiment name
        <input
          required
          maxLength={128}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </label>
      <label>
        Full experiment configuration
        <textarea
          required
          rows={16}
          spellCheck={false}
          value={text}
          onChange={(e) => setText(e.target.value)}
        />
      </label>
      <p className="muted">
        Declare training budgets, development and final seeds, and confirmation
        criteria before starting. A previously reserved final schedule becomes
        development data automatically.
      </p>
      {error && (
        <p className="alert" role="alert">
          {error}
        </p>
      )}
      <div className="actions">
        <button className="primary" disabled={!defaults || busy}>
          Start experiment
        </button>
        <button
          type="button"
          disabled={!defaults || busy}
          onClick={() => {
            if (defaults) setText(JSON.stringify(defaults, null, 2));
          }}
        >
          Restore five-seed defaults
        </button>
      </div>
    </form>
  );
}
function ResearchDetail({ id }: { id: string }) {
  const [x, setX] = useState<Experiment | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [retry, setRetry] = useState(0);
  useEffect(() => {
    let alive = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const refresh = async () => {
      try {
        const next = await api<Experiment>(`/experiments/${id}`);
        if (alive) {
          setX((old) => (!old || next.revision >= old.revision ? next : old));
          setError("");
          if (researchActive(next)) timer = setTimeout(refresh, 1000);
        }
      } catch (e) {
        if (alive) {
          setError(failure(e));
          timer = setTimeout(refresh, 3000);
        }
      }
    };
    void refresh();
    return () => {
      alive = false;
      clearTimeout(timer);
    };
  }, [id, retry]);
  async function control(action: "stop" | "resume") {
    if (!x || busy) return;
    setBusy(true);
    try {
      const next = await api<Experiment>(
        `/experiments/${id}/${action}`,
        "POST",
        command(x.version),
      );
      setX((old) => (!old || next.revision >= old.revision ? next : old));
      setRetry((v) => v + 1);
    } catch (e) {
      setError(failure(e));
      setRetry((v) => v + 1);
    } finally {
      setBusy(false);
    }
  }
  if (!x)
    return (
      <>
        <a className="back-link" href="#/research">
          ← Experiments
        </a>
        <p role="status">Loading experiment…</p>
        {error && (
          <p role="alert" className="alert">
            {error}
          </p>
        )}
      </>
    );
  const runs = x.runs ?? [];
  const target = x.config.training_seeds.length * x.config.methods.length;
  return (
    <>
      <a className="back-link" href="#/research">
        ← Experiments
      </a>
      <div className="game-heading">
        <div>
          <p className="eyebrow">MULTI-SEED RESEARCH</p>
          <h1>{x.name}</h1>
        </div>
        <span className="tag" data-testid="research-state">
          {x.state}
        </span>
      </div>
      {error && (
        <p role="alert" className="alert">
          {error}
        </p>
      )}
      {x.error && (
        <p role="alert" className="alert">
          {x.error}
        </p>
      )}
      <section className="lab-panel lab-form">
        <div className="section-heading">
          <h2>{x.phase}</h2>
          <span>
            {runs.length} / {target} seed runs
          </span>
        </div>
        <progress
          aria-label="Completed research seed runs"
          value={runs.length}
          max={target}
        />
        <p>
          {x.counters.games} physical games ·{" "}
          {x.counters.decisions.toLocaleString("en")} decisions ·{" "}
          {x.counters.forward_evaluations.toLocaleString("en")} forward
          evaluations · {x.counters.updates ?? 0} updates
        </p>
        <p>
          {x.wall_seconds.toFixed(3)} s measured trainer/evaluator time. Queue,
          storage and publication time are excluded; CPU time is not measured.
        </p>
        {x.active && (
          <p>
            Current: {methodName(x.active.algorithm)} · seed{" "}
            {seedLabel(x.active.seed)} · {x.active.training_games} training +{" "}
            {x.active.selection_games} selection games ·{" "}
            {x.active.development_counters.games} development games
          </p>
        )}
        <div className="actions">
          {researchActive(x) && (
            <button
              disabled={busy || x.state === "stopping"}
              onClick={() => control("stop")}
            >
              Stop experiment
            </button>
          )}
          {x.can_resume && (
            <button
              disabled={busy}
              className="primary"
              onClick={() => control("resume")}
            >
              Resume experiment
            </button>
          )}
          <button onClick={() => setRetry((v) => v + 1)}>
            Refresh experiment
          </button>
          {x.report_sha256 && (
            <a className="button" href={`/api/experiments/${x.id}/report`}>
              Download research report
            </a>
          )}
        </div>
      </section>
      {!x.config.training_seeds.every(Number.isSafeInteger) && (
        <p className="alert">
          Some seed values exceed browser numeric precision. Use the downloaded
          report for exact values.
        </p>
      )}
      <section className="research-results">
        <div className="section-heading">
          <h2>Development comparison</h2>
          <span>
            {percent(x.config.bootstrap.confidence)} percentile intervals
          </span>
        </div>
        <p>
          Each dot is an independent training seed. Budgets include all learners
          and selection games, including discarded participants. Development
          evaluation is charged separately below.
        </p>
        {runs.length ? (
          <>
            <div className="research-charts">
              <BudgetChart runs={runs} axis="decisions" />
              <BudgetChart runs={runs} axis="time" />
            </div>
            <div className="research-legend">
              {x.config.methods.map((m, i) => (
                <span key={m.algorithm}>
                  <i style={{ background: colors[i % colors.length] }} />
                  {methodName(m.algorithm)}
                </span>
              ))}
            </div>
            <SeedTable runs={runs} />
          </>
        ) : (
          <p className="empty-state">
            Charts appear when actual seed runs finish development evaluation.
          </p>
        )}
        {(x.methods ?? []).length > 0 && (
          <div className="research-table">
            <table>
              <caption>All-seed method estimates</caption>
              <thead>
                <tr>
                  <th>Method</th>
                  <th>Seeds / pairs</th>
                  <th>Games</th>
                  <th>Win rate</th>
                  <th>Interval</th>
                  <th>Mean points</th>
                  <th>Mars wins</th>
                </tr>
              </thead>
              <tbody>
                {x.methods!.map((m) => (
                  <tr key={m.algorithm}>
                    <th>{methodName(m.algorithm)}</th>
                    <td>
                      {m.estimate.training_seeds} / {m.estimate.pairs}
                    </td>
                    <td>{m.estimate.games}</td>
                    <td>{percent(m.estimate.win_rate)}</td>
                    <td>{interval(m.estimate)}</td>
                    <td>{m.estimate.mean_points.toFixed(3)}</td>
                    <td>{percent(m.estimate.mars_win_rate)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        {(x.differences ?? []).length > 0 && (
          <div className="research-table">
            <table>
              <caption>Paired method differences (A − B)</caption>
              <thead>
                <tr>
                  <th>Methods</th>
                  <th>Win-rate difference</th>
                  <th>Interval</th>
                </tr>
              </thead>
              <tbody>
                {x.differences!.map((d) => (
                  <tr key={d.a + d.b}>
                    <th>
                      {methodName(d.a)} − {methodName(d.b)}
                    </th>
                    <td>{percent(d.estimate.win_rate)}</td>
                    <td>{interval(d.estimate)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
        <p className="muted">
          Pairs are resampled together within opponent strata; training seed
          clusters are resampled above them. Few seeds and degenerate samples
          limit the evidence. Method differences are not head-to-head
          probabilities.
        </p>
      </section>
      <section className="lab-panel lab-form">
        <h2>Final confirmation</h2>
        <p>
          {x.selection_locked
            ? "Candidate locked before the first final game."
            : "Selection will use development results only; final results cannot choose another candidate."}
        </p>
        <p>
          {x.final_data_role === "heldout"
            ? "Held-out schedule"
            : "Reused schedule: development data, not independent confirmation"}{" "}
          · {x.final_counters.games} / {x.config.confirmation.pairs * 2} games ·{" "}
          {x.final_seconds.toFixed(3)} s
        </p>
        <p>
          Incumbent:{" "}
          <code className="research-id">{x.config.incumbent_id}</code>. Requires
          at least {x.config.confirmation.minimum_pairs} independent pairs, a
          nondegenerate bootstrap and a lower confidence bound above{" "}
          {percent(0.5 + x.config.confirmation.minimum_margin)}.
        </p>
        {x.verdict && (
          <div data-testid="research-verdict">
            <strong>
              {x.verdict.status === "confirmed"
                ? "Confirmed against the declared incumbent"
                : "Candidate retained"}
            </strong>
            <p>{x.verdict.reason}</p>
            <p>
              {percent(x.verdict.estimate.win_rate)} wins · interval{" "}
              {interval(x.verdict.estimate)} · {x.verdict.estimate.pairs} pairs
            </p>
          </div>
        )}
        {x.candidate_id && (
          <p>
            Frozen candidate:{" "}
            <code className="research-id">{x.candidate_id}</code>
            <br />
            <a href={`#/evaluations?bot=${x.candidate_id}`}>
              Evaluate saved candidate
            </a>{" "}
            · <a href="#">Open My bots to play</a>
          </p>
        )}
        {x.final_data_reused_from.length > 0 && (
          <p>Previously reserved by: {x.final_data_reused_from.join(", ")}</p>
        )}
      </section>
      <details className="lab-panel lab-form">
        <summary>Exact configuration and execution contracts</summary>
        <p>
          {x.execution.platform} · {x.execution.go_version} · source{" "}
          {x.execution.source_revision}
          {x.execution.source_modified ? " (modified)" : ""}
        </p>
        <pre>
          {JSON.stringify(
            {
              config: x.config,
              execution: x.execution,
              report_sha256: x.report_sha256,
            },
            null,
            2,
          )}
        </pre>
      </details>
    </>
  );
}
export function BudgetChart({
  runs,
  axis,
}: {
  runs: ResearchRun[];
  axis: "decisions" | "time";
}) {
  const valid = runs.filter((r) => r.estimate !== null);
  const names = [...new Set(runs.map((r) => r.algorithm))];
  const value = (r: ResearchRun) =>
    axis === "decisions" ? r.counters.decisions : r.wall_seconds;
  const max = Math.max(1, ...valid.map(value));
  const X = (v: number) => 52 + (v / max) * 410;
  const Y = (v: number) => 200 - v * 164;
  return (
    <figure className="research-chart">
      <figcaption>
        {axis === "decisions"
          ? "Environment interactions"
          : "Measured training + selection time"}
      </figcaption>
      <svg
        viewBox="0 0 500 245"
        role="img"
        aria-label={`${axis === "decisions" ? "Decisions" : "Training time"} versus development win rate; every training seed`}
      >
        {[0, 0.5, 1].map((v) => (
          <g key={v}>
            <line x1={52} x2={462} y1={Y(v)} y2={Y(v)} className="chart-grid" />
            <text x={45} y={Y(v) + 4} textAnchor="end">
              {percent(v)}
            </text>
          </g>
        ))}
        {[0, max / 2, max].map((v, i) => (
          <text key={i} x={X(v)} y={219} textAnchor="middle">
            {axis === "time"
              ? `${v.toFixed(2)} s`
              : Math.round(v).toLocaleString("en")}
          </text>
        ))}
        {valid.map((r) => (
          <g
            key={r.algorithm + r.seed}
            data-seed={seedLabel(r.seed)}
            data-algorithm={r.algorithm}
            data-budget={value(r)}
          >
            <title>
              {methodName(r.algorithm)} · seed {seedLabel(r.seed)}:{" "}
              {percent(r.estimate!.win_rate)}, interval {interval(r.estimate!)},{" "}
              {value(r).toFixed(3)} {axis === "time" ? "s" : "decisions"}
            </title>
            <line
              x1={X(value(r))}
              x2={X(value(r))}
              y1={Y(r.estimate!.win_interval.low)}
              y2={Y(r.estimate!.win_interval.high)}
              stroke={colors[names.indexOf(r.algorithm) % colors.length]}
              strokeWidth={2}
            />
            <circle
              cx={X(value(r))}
              cy={Y(r.estimate!.win_rate)}
              r={5}
              fill={colors[names.indexOf(r.algorithm) % colors.length]}
            />
          </g>
        ))}
      </svg>
    </figure>
  );
}
function SeedTable({ runs }: { runs: ResearchRun[] }) {
  return (
    <div className="research-table">
      <table>
        <caption>Every training seed and measured work</caption>
        <thead>
          <tr>
            <th>Method / seed</th>
            <th>Training / selection games</th>
            <th>Decisions</th>
            <th>Forwards / updates</th>
            <th>Training time</th>
            <th>Development games / time</th>
            <th>Win rate</th>
            <th>Model</th>
          </tr>
        </thead>
        <tbody>
          {runs.map((r) => (
            <tr key={r.algorithm + r.seed}>
              <th>
                {methodName(r.algorithm)} / {seedLabel(r.seed)}
              </th>
              <td>
                {r.training_games} / {r.selection_games}
              </td>
              <td>{r.counters.decisions}</td>
              <td>
                {r.counters.forward_evaluations} / {r.counters.updates ?? 0}
              </td>
              <td>{r.wall_seconds.toFixed(3)} s</td>
              <td>
                {r.development_counters.games} /{" "}
                {r.development_seconds.toFixed(3)} s
              </td>
              <td>{r.estimate ? percent(r.estimate.win_rate) : "—"}</td>
              <td>
                <a title={r.bot_id} href={`#/evaluations?bot=${r.bot_id}`}>
                  {r.bot_id.slice(0, 10)}…
                </a>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
