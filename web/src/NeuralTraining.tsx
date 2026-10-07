import { useState } from "react";
import { api, command } from "./api";
import type { Bot } from "./api";
import { algorithmName, failure } from "./jobs";
import type { Job, TDMetric } from "./jobs";

export function TDChart({ history }: { history: TDMetric[] }) {
  if (!history.length)
    return (
      <p className="empty-state">
        TD error appears after the first game boundary is saved.
      </p>
    );
  // Keep rendering bounded while retaining the full history in the job snapshot.
  const points = history.slice(-200);
  const maximum = Math.max(...points.map((m) => m.mean_abs_delta)) || 1;
  const x = (i: number) => 45 + (i * 560) / Math.max(1, points.length - 1);
  const y = (value: number) => 170 - (value / maximum) * 150;
  return (
    <>
      <svg
        className="fitness-chart"
        viewBox="0 0 640 210"
        role="img"
        aria-label="Mean absolute TD error per saved game"
      >
        {[0, maximum / 2, maximum].map((value) => (
          <g key={value}>
            <line
              x1={45}
              x2={605}
              y1={y(value)}
              y2={y(value)}
              className="chart-grid"
            />
            <text x={5} y={y(value) + 5}>
              {value.toPrecision(2)}
            </text>
          </g>
        ))}
        <polyline
          className="chart-best"
          points={points
            .map((m, i) => `${x(i)},${y(m.mean_abs_delta)}`)
            .join(" ")}
        />
        {points.map((m, i) => (
          <circle
            key={m.game}
            cx={x(i)}
            cy={y(m.mean_abs_delta)}
            r={3}
            className="chart-dot"
          >
            <title>{`Game ${m.game}: mean absolute TD error ${m.mean_abs_delta.toFixed(6)}, ${m.updates} updates, ${m.status}`}</title>
          </circle>
        ))}
        <text x={45} y={198}>
          Game {points[0].game}
        </text>
        <text x={605} y={198} textAnchor="end">
          {points.length > 1 ? `Game ${points.at(-1)!.game}` : ""}
        </text>
      </svg>
      <p className="muted">
        Actual mean absolute TD error across each game's updates. This is a
        learning diagnostic; use independent evaluation to measure game results.
        {history.length > points.length ? " Showing the latest 200 games." : ""}
      </p>
    </>
  );
}

export function NeuralSnapshot({
  x,
  refresh,
}: {
  x: Job;
  refresh: () => Promise<void>;
}) {
  const [name, setName] = useState("My neural bot");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState<Bot | null>(null);
  const candidate = x.neural_candidate;
  return (
    <section className="lab-panel">
      <h2>Neural snapshot</h2>
      <p className="muted">
        {candidate
          ? `Save ${algorithmName(x)} weights from game ${candidate.game}. The saved copy stays fixed while learning continues.`
          : "Finish the first game to save a neural snapshot."}
      </p>
      <form
        className="name-form"
        onSubmit={async (event) => {
          event.preventDefault();
          if (!candidate) return;
          setBusy(true);
          setError("");
          setNotice(null);
          try {
            setNotice(
              await api<Bot>(`/training/runs/${x.id}/save-bot`, "POST", {
                ...command(x.version),
                generation: candidate.game,
                candidate_id: candidate.id,
                name,
              }),
            );
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
        <button className="primary" disabled={busy || !candidate}>
          Save neural snapshot
        </button>
      </form>
      {error && (
        <p className="alert" role="alert">
          {error}
        </p>
      )}
      {notice && (
        <p className="notice" role="status">
          Saved as {notice.name}. Identical strategies share one snapshot.{" "}
          <a className="button" href="#/bots">
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
