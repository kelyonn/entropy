# UI System — Slate Studio

Slate Studio is the Entropy design language. It is a premium, calm, easy-to-scan
product chrome in light tones, with exactly one dark surface: the terminal well.
This document is the target definition. It is written in Stage 0 and implemented
in Stage 2. After Stage 2, this document must match the shipped implementation.

## Principles
- One composition. The first viewport reads as a single product surface, not a
  busy dashboard.
- Brand-first. The ENTROPY wordmark is the strongest signal in the header.
- Calm and technical. Short, precise copy. No marketing fluff.
- No emojis anywhere.
- Light chrome, dark terminal only. Do not darken the whole product.

## Explicitly rejected looks
- Purple or indigo gradients
- Neon glow effects
- Emoji icons
- Rounded-full pill clusters
- Multi-layer drop shadows
- Generic Inter-on-flat-black dashboard aesthetic

## Color tokens

| Token | Value | Use |
|-------|-------|-----|
| `--canvas` | `#F5F6F8` | Page background |
| `--surface` | `#FFFFFF` | Panels |
| `--surface-muted` | `#EEF0F3` | Secondary wells |
| `--ink` | `#14181F` | Primary text |
| `--ink-muted` | `#5C6570` | Secondary text |
| `--border` | `#E2E5EA` | Hairlines |
| `--accent` | `#1F6B5C` | Primary actions, brand mark |
| `--accent-soft` | `#E6F2EF` | Soft highlight |
| `--danger` | `#B42318` | Crash, error |
| `--warning` | `#A15C07` | Analyzing |
| `--success` | `#1B7A4E` | Healthy, connected |
| `--radius` | `8px` | Panel radius (not pills) |
| Terminal bg | `#0E1116` | Xterm only |
| Terminal fg | `#D7DCE2` | Xterm only |

## Typography
- UI sans: `"IBM Plex Sans", system-ui, sans-serif`
- Mono (terminal, session ids): `"IBM Plex Mono", ui-monospace, monospace`

## Layout (>=1024px)

```text
+-------------------------------------------------------------+
| ENTROPY  tagline     status . session . clock . snapshot     |
+--------------------------------------+----------------------+
| Terminal (dark well, ~65%)           | Labs / Mission panel |
|                                      |                      |
+--------------------------------------+----------------------+
| Activity timeline                    | System view: 3D gauge|
|                                      | rack + CPU/mem/PIDs  |
+--------------------------------------+----------------------+
| Explanation drawer (collapsible, full width)                |
+-------------------------------------------------------------+
```

- Labs panel: Incidents / Demos tabs, progress bar, rows with category,
  difficulty, minutes, and best result. Selecting a row opens a briefing dialog.
- Mission panel replaces the catalog while a lab runs: briefing, objective
  checklist (ticks live), revealed hints, Check / Hint / Nudge / Abandon, and
  a debrief on completion.
- System view: three columns (CPU, memory, PIDs) inside capacity outlines,
  colored by threshold (accent < 60%, warning < 85%, danger), with sparklines.
- Status marks are small square SVG glyphs, never emoji or round pills.
- System view has two tabs: Resources (3D gauge rack, gauges, production
  traffic during stack labs) and Processes (live pstree; new rows flash accent,
  exited rows fade in danger, zombies in warning, OOM victim labeled).
- Game day shows a banner above the mission with an error-budget bar
  (accent > 50%, warning > 20%, danger below).
- Interview missions show a countdown instead of elapsed time and a letter
  grade card on completion.
- Replay page (`/?replay=token`) is a read-only player: terminal well on the
  left, milestones on the right, scrubber and 1x/2x/4x speed.

- Header: wordmark as the hero signal, a short tagline, and one status line on
  the right (connected / reconnecting / offline) using text plus color.
- Terminal: dark well on the left, roughly 60% width.
- System view: light panel on the right, roughly 40% width, holding the 3D
  visualization and a simple metrics row.
- AI drawer: collapsible bottom panel titled "Explanation".

## Component notes
- Metrics: plain numeric cards, no icon glyphs.
- Status: text plus color, never an emoji indicator.
- AI drawer states: empty, analyzing (text plus a subtle CSS spinner),
  streaming, complete, fallback error.
- Buttons: text labels, `--radius` corners, accent for primary actions. No
  pill shapes.

## Motion (2-3 intentional only)
1. Status bar fade on connection-state change.
2. AI drawer height expand and collapse.
3. Visualization color transition on health change (subtle).

## Visualization color mapping
- Healthy: `--success`
- Error / non-zero exit: `--danger`
- Analyzing: `--warning`
- Subtle shake on OOM; stronger motion only on a genuine crash. No emoji
  overlays on the canvas.
