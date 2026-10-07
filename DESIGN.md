---
name: EvoNardy
description: A calm local long-nardy table with room to train opponents.
colors:
  canvas: "#f5f2e9"
  surface: "#fffcf5"
  ink: "#243c32"
  muted: "#5b685d"
  line: "#d6d7c9"
  accent: "#a3442d"
  accent-hover: "#873722"
  button-hover: "#e5e9dc"
  nav-hover: "#e9ebdf"
  panel: "#e8ecdf"
  input-border: "#8b9989"
  board-frame: "#243e34"
  board-field: "#e5ddc5"
  point-warm: "#b48b5b"
  point-green: "#355849"
  checker-white: "#ece7d6"
  checker-black: "#10221c"
  destination: "#e8b755"
typography:
  body:
    fontFamily: '"Avenir Next", "Segoe UI", sans-serif'
    lineHeight: 1.5
  display:
    fontSize: "clamp(28px, 3vw, 40px)"
    fontWeight: 600
    lineHeight: 1.12
    letterSpacing: "-0.035em"
  title:
    fontSize: "25px"
    fontWeight: 600
    letterSpacing: "-0.035em"
  navigation:
    fontSize: "14px"
  form-label:
    fontSize: "12px"
  research-code:
    fontFamily: "ui-monospace, monospace"
    fontSize: "0.85rem"
    lineHeight: 1.5
rounded:
  tag: "5px"
  field: "6px"
  control: "8px"
  panel: "10px"
spacing:
  compact: "8px"
  small: "12px"
  medium: "16px"
  field-gap: "18px"
  large: "24px"
  layout: "32px"
components:
  button-primary:
    backgroundColor: "{colors.accent}"
    textColor: "{colors.surface}"
    rounded: "{rounded.control}"
    padding: "9px 16px"
  button-primary-hover:
    backgroundColor: "{colors.accent-hover}"
  button-secondary:
    backgroundColor: "transparent"
    textColor: "{colors.ink}"
    rounded: "{rounded.control}"
    padding: "9px 16px"
  button-text:
    backgroundColor: "transparent"
    textColor: "{colors.accent}"
    padding: "2px 0"
  input:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.field}"
    padding: "8px 12px"
  navigation-active:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.surface}"
    rounded: "{rounded.field}"
    padding: "10px 22px"
  tag:
    textColor: "{colors.muted}"
    rounded: "{rounded.tag}"
    padding: "5px 9px"
  card:
    backgroundColor: "{colors.surface}"
    rounded: "{rounded.control}"
    padding: "24px"
  play-panel:
    backgroundColor: "{colors.panel}"
    rounded: "{rounded.panel}"
    padding: "30px 24px 24px"
---

# Design System: EvoNardy

## Overview

**Creative North Star: "Your place at the board"**

A warm, calm light shell surrounds a contrasting dark green long-nardy board. The board carries the visual weight; compact navigation, quiet dividers and a restrained terracotta action color keep the next move clear.

Training uses the same palette and typography with denser forms and real model data. The interface is desktop-first and stacks into a single column on small screens.

**Key Characteristics:**

- Board-led play with a compact action sidebar.
- Warm paper surfaces, forest green ink and terracotta actions.
- Quiet containers, readable forms and visible keyboard focus.

## Colors

### Primary

Terracotta (`accent`) identifies primary actions, links, selected checker rings and keyboard focus. Its darker hover value gives buttons a clear response.

### Secondary

Forest green frames the SVG board; warm and green points alternate on its pale field. Cream and near-black checkers remain distinct. Gold marks legal destinations.

### Neutral

Warm paper (`canvas`) surrounds cream surfaces. Green ink and muted text provide hierarchy; pale borders separate sections. The play sidebar uses a soft sage panel.

## Typography

Avenir Next, with Segoe UI and sans-serif fallbacks, carries the interface. Headings are moderately weighted with tight tracking; navigation and form labels remain compact. The Play headline uses the display role; Training and Settings titles use (38px), while the active-game title uses (30px). Play drops to (28px) on small screens.

Research configuration uses the monospace role. Training statistics, research tables and move history use tabular numerals. Game help is limited to (70ch). Decorative eyebrows are absent from page headings.

## Layout

The centered shell is capped at (1440px), with (48px) desktop side padding and a compact header of at least (88px). Play and active games use a flexible board column, a (292px) sidebar and a (32px) gap. At (1100px), side padding becomes (28px), the sidebar (260px) and the gap (24px).

At (800px), board and sidebar stack; navigation becomes a full-width second header row. At (600px), side padding becomes (18px), Settings controls stack, and quarter labels, point numbers and stack counts enlarge. The SVG scales with its container. Long identifiers wrap; research tables and code blocks own any necessary horizontal scrolling.

Training retains Bots, Runs, Evaluate and Research tabs. Bot cards use three columns, two at (980px) and one at (600px); forms use three columns and two at (600px). Research charts stack at (700px). Settings has an (850px) content cap.

## Elevation & Depth

Tonal surfaces and thin borders define nearly all depth. The board alone receives a soft drop shadow (`0 10px 10px #243c321c`); cards and panels stay flat.

## Shapes

Controls and training containers use gently rounded corners. Fields and active navigation have tighter corners; the play panel is slightly softer. Checkers, player-color markers and status dots remain circular. The board retains its existing SVG geometry.

## Components

### Buttons

Primary buttons pair terracotta with cream text and bold weight. Secondary buttons are transparent with a thin border; text actions are underlined. Standard controls have a (42px) minimum height, start actions (48px), and the main in-game action (46px). Hover changes background over (0.15s). Disabled buttons use (0.55) opacity and a not-allowed cursor. No separate pressed-state styling is defined.

Keyboard focus uses a (3px) terracotta outline with (4px) offset. SVG point focus instead thickens the triangle stroke to (5px); checker selection uses a (4px) ring.

### Inputs / Fields

Cream fields have a subdued green border, compact padding and a (42px) minimum height. Labels sit above controls. Inputs and textareas use a terracotta caret and muted placeholders; controls share the global focus treatment. Research textareas resize vertically.

### Navigation

Play, Training and Settings sit in the header. The active destination is filled green with cream text; inactive destinations have muted text and a pale hover surface. Training tabs use a terracotta underline and heavier active text.

### Cards / Containers

Bot cards and training panels use cream surfaces, thin dividers and no shadow. The sage play panel groups the opponent, preferred color and primary action. The active-game sidebar uses a divider, stacked turn controls and a bounded scrollable move history.

### Tags

Small outlined tags use muted text, a compact radius and lightly tracked type. They remain secondary to names and actions.

### Long-nardy board

The existing SVG is the signature surface, not a decorative image. Legal source, destination, selection and keyboard focus states remain distinct. Heuristic is the initial opponent, later choices are remembered, and a saved game opens its real board through Continue. Settings owns preferred checker color. Go remains the source of legal continuations. Each quarter has its own head or home label. A white arrow above the field points left; a black arrow below it points right. Head and home labels sit outside the board. Borne-off counters sit beside the corresponding homes. Opponent steps highlight the source checker in yellow for one second, then the destination checker in blue for one second, with the used die displayed. Human dice roll automatically after playback ends.

## Do's and Don'ts

### Do:

- Do keep the board dominant and the next game action easy to find.
- Do use real saved boards, opponents and training results.
- Do retain visible keyboard focus and wrapping for long identifiers.
- Do preserve dense working forms in Training.

### Don't:

- Don't add decorative eyebrows above page titles.
- Don't use fabricated difficulty labels or training metrics.
- Don't let the page overflow horizontally on mobile; wide research tables scroll inside their container.
