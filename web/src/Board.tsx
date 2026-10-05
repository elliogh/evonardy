import { useId } from "react";
import type { Position, Step } from "./api";
import { color } from "./api";

const fieldX = 24;
const fieldWidth = 852;
const dividerWidth = 40;
const halfWidth = (fieldWidth - dividerWidth) / 2;
const pointSpacing = halfWidth / 6;

type Props = {
  position: Position;
  next: Step[];
  selected: number | null;
  disabled: boolean;
  onPoint: (point: number) => void;
  highlight?: Step;
  highlightStage?: "source" | "destination";
};
export function Board({
  position,
  next,
  selected,
  disabled,
  onPoint,
  highlight,
  highlightStage = "destination",
}: Props) {
  const routeID = useId();
  const leftQuarter = fieldX + halfWidth / 2;
  const rightQuarter = fieldX + halfWidth + dividerWidth + halfWidth / 2;
  const whiteRoute = "M850 28H50";
  const blackRoute = "M50 598H850";
  const sources = new Set(next.map((step) => step.from));
  const destinations = new Set(
    next.filter((step) => step.from === selected).map((step) => step.to),
  );
  return (
    <div className="board-wrap">
      <svg
        className="board"
        data-playback-stage={highlight ? highlightStage : undefined}
        viewBox="-74 -38 1048 692"
        role="group"
        aria-label="Long nardy board, points increase counterclockwise"
      >
        <rect
          className="board-frame"
          x="1"
          y="1"
          width="898"
          height="618"
          rx="24"
        />
        <rect
          className="board-field"
          x={fieldX}
          y="54"
          width={fieldWidth}
          height="512"
          rx="10"
        />
        <rect
          className="board-bar"
          x={fieldX + halfWidth}
          y="54"
          width={dividerWidth}
          height="512"
        />
        <defs>
          <marker
            id={`${routeID}-white`}
            viewBox="-1 -1 12 12"
            refX="9"
            refY="5"
            markerWidth="4"
            markerHeight="4"
            orient="auto"
          >
            <path
              d="M0 0L10 5L0 10Z"
              fill="#ece7d6"
              stroke="#b7af96"
              strokeWidth="1"
            />
          </marker>
          <marker
            id={`${routeID}-black`}
            viewBox="-1 -1 12 12"
            refX="9"
            refY="5"
            markerWidth="4"
            markerHeight="4"
            orient="auto"
          >
            <path
              d="M0 0L10 5L0 10Z"
              fill="#10221c"
              stroke="#789582"
              strokeWidth="1"
            />
          </marker>
        </defs>
        <g
          className="board-routes"
          role="img"
          aria-label="White moves right to left across the top. Black moves left to right across the bottom."
        >
          <path className="board-route white-route-outline" d={whiteRoute} />
          <path
            className="board-route white-route"
            d={whiteRoute}
            markerEnd={`url(#${routeID}-white)`}
          />
          <path className="board-route black-route-outline" d={blackRoute} />
          <path
            className="board-route black-route"
            d={blackRoute}
            markerEnd={`url(#${routeID}-black)`}
          />
        </g>
        <text
          className="board-quarter-label"
          textAnchor="middle"
          x={leftQuarter}
          y="-14"
        >
          Black home
        </text>
        <text
          className="board-quarter-label"
          textAnchor="middle"
          x={rightQuarter}
          y="-14"
        >
          White head
        </text>
        <text
          className="board-quarter-label lower"
          textAnchor="middle"
          x={leftQuarter}
          y="645"
        >
          Black head
        </text>
        <text
          className="board-quarter-label lower"
          textAnchor="middle"
          x={rightQuarter}
          y="645"
        >
          White home
        </text>
        {Array.from({ length: 24 }, (_, point) => {
          const top = point < 12;
          const column = top ? 11 - point : point - 12;
          const x =
            fieldX +
            (column + 0.5) * pointSpacing +
            (column >= 6 ? dividerWidth : 0);
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
              {highlight &&
                count > 0 &&
                (highlightStage === "source"
                  ? highlight.from
                  : highlight.to) === point && (
                  <circle
                    className="playback-checker-ring"
                    cx={x}
                    cy={
                      y + (top ? 1 : -1) * (24 + (Math.min(count, 5) - 1) * 42)
                    }
                    r="26"
                  />
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
        {highlightStage === "destination" && highlight?.to === 24 && (
          <circle
            className="playback-checker-ring"
            cx={position.turn === 0 ? 940 : -40}
            cy={position.turn === 0 ? 444 : 176}
            r="26"
          />
        )}
        <g role="img" aria-label={`White borne off: ${position.borne_off[0]}`}>
          <circle className="checker white" cx="940" cy="444" r="23" />
          <text className="stack-count on-white" x="940" y="451">
            {position.borne_off[0]}
          </text>
        </g>
        <g role="img" aria-label={`Black borne off: ${position.borne_off[1]}`}>
          <circle className="checker black" cx="-40" cy="176" r="23" />
          <text className="stack-count on-black" x="-40" y="183">
            {position.borne_off[1]}
          </text>
        </g>
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
