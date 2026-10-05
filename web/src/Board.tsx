import type { Position, Step } from "./api";
import { color } from "./api";
type Props = {
  position: Position;
  next: Step[];
  selected: number | null;
  disabled: boolean;
  onPoint: (point: number) => void;
};
export function Board({ position, next, selected, disabled, onPoint }: Props) {
  const sources = new Set(next.map((step) => step.from));
  const destinations = new Set(
    next.filter((step) => step.from === selected).map((step) => step.to),
  );
  return (
    <div className="board-wrap">
      <svg
        className="board"
        viewBox="0 0 960 620"
        role="group"
        aria-label="Long nardy board, points increase counterclockwise"
      >
        <rect
          className="board-frame"
          x="1"
          y="1"
          width="958"
          height="618"
          rx="24"
        />
        <rect
          className="board-field"
          x="24"
          y="54"
          width="852"
          height="512"
          rx="10"
        />
        <path className="board-bar" d="M450 54h40v512h-40z" />
        <text className="board-direction" x="40" y="33">
          ← Black home · White head
        </text>
        <text className="board-direction" x="40" y="598">
          Black head · White home →
        </text>
        {Array.from({ length: 24 }, (_, point) => {
          const top = point < 12;
          const column = top ? 11 - point : point - 12;
          const x = 28 + column * 66 + (column >= 6 ? 40 : 0) + 33;
          const y = top ? 66 : 554;
          const count =
            position.checkers[0][point] + position.checkers[1][point];
          const player = position.checkers[0][point] > 0 ? 0 : 1;
          const active =
            !disabled && (destinations.has(point) || sources.has(point));
          const label = `Point ${point + 1}, ${count ? `${count} ${color(player)} checkers` : "empty"}${active ? (destinations.has(point) ? ", move here" : ", select checker") : ""}`;
          return (
            <g
              key={point}
              data-point={point}
              role="button"
              aria-label={label}
              aria-disabled={!active}
              tabIndex={active ? 0 : -1}
              className={`point ${point % 2 ? "light" : "dark"} ${active ? "active" : ""} ${selected === point ? "selected" : ""} ${destinations.has(point) ? "destination" : ""}`}
              onClick={() => active && onPoint(point)}
              onKeyDown={(event) => {
                if (active && (event.key === "Enter" || event.key === " ")) {
                  event.preventDefault();
                  onPoint(point);
                }
              }}
            >
              <rect
                className="point-hit"
                x={x - 32}
                y={top ? 54 : 322}
                width="64"
                height="244"
              />
              <path
                className="triangle"
                d={`M${x - 30} ${y} L${x + 30} ${y} L${x} ${top ? 292 : 328} Z`}
              />
              <text className="point-number" x={x} y={top ? 49 : 580}>
                {point + 1}
              </text>
              {Array.from({ length: Math.min(count, 5) }, (_, i) => (
                <circle
                  key={i}
                  className={`checker ${player === 0 ? "white" : "black"}`}
                  cx={x}
                  cy={y + (top ? 1 : -1) * (24 + i * 42)}
                  r="23"
                />
              ))}
              {count > 0 && (
                <text
                  className={`stack-count ${player === 0 ? "on-white" : "on-black"}`}
                  x={x}
                  y={y + (top ? 1 : -1) * 24 + 6}
                >
                  {count}
                </text>
              )}
              {selected === point && (
                <circle
                  className="selection-ring"
                  cx={x}
                  cy={y + (top ? 1 : -1) * 24}
                  r="28"
                />
              )}
            </g>
          );
        })}
        <text className="off-label" x="919" y="112" textAnchor="middle">
          OFF
        </text>
        <circle className="checker white" cx="919" cy="156" r="23" />
        <text className="stack-count on-white" x="919" y="163">
          {position.borne_off[0]}
        </text>
        <circle className="checker black" cx="919" cy="464" r="23" />
        <text className="stack-count on-black" x="919" y="471">
          {position.borne_off[1]}
        </text>
      </svg>
      <div className="off-summary">
        <span>White off: {position.borne_off[0]}</span>
        <span>Black off: {position.borne_off[1]}</span>
        {destinations.has(24) && (
          <button disabled={disabled} onClick={() => onPoint(24)}>
            Bear off
          </button>
        )}
      </div>
    </div>
  );
}
