# ADR 0002: Slate Studio UI (light chrome, dark terminal only)

Status: accepted
Date: 2026-07-17

## Context
The MVP UI is a dark, emoji-heavy Tailwind layout that reads as a generic
hackathon dashboard. The product needs to feel premium, calm, and easy to use,
and it must avoid the common AI-generated design clusters (purple gradients,
neon glow, cream-and-serif, dense broadsheet).

## Decision
Adopt "Slate Studio": a premium light product chrome with a single dark surface,
the terminal well. Use a deep teal accent, IBM Plex Sans for UI and IBM Plex
Mono for terminal and identifiers, hairline borders, 8px panel radius, and 2-3
intentional motions. Prohibit emojis everywhere. Full tokens and layout live in
`docs/ui.md`.

## Consequences
- The entire frontend chrome is rebuilt in Stage 2, not merely restyled.
- All emoji are removed from `frontend/src/**` and user-facing copy.
- The terminal keeps a dark background for readability while the rest of the
  product stays light.
- New shared components (app shell, status bar, metric tile) may be introduced
  to keep `App.jsx` small.
