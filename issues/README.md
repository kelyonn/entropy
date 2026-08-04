# Issues

This folder is a lightweight, in-repo issue tracker so that context survives
across sessions without depending on an external tool.

## Structure
- `templates/` — copy these when filing a new issue (`bug.md`, `feature.md`,
  `spike.md`).
- `backlog/` — active PRD delivery checklists `P00-*.md` through `P12-*.md`.
  Legacy MVP stages `S00-*.md` through `S13-*.md` are complete history.
  Ad hoc issues can also live here using the templates.

## How to use
1. When starting a stage, open its `backlog/PXX-*.md` and work the checklist.
2. Check items off as you complete them. Do not mark a stage done until every
   exit-criteria item is checked and verified (AWS-blocked exits stay unchecked).
3. For a new bug or feature outside the stage checklist, copy a template into
   `backlog/` with a short kebab-case name, for example
   `backlog/fix-websocket-reconnect.md`.
4. Keep entries short and factual. No emojis.

## Conventions
- Title format: `PXX: short description` for active stages (`SXX` for legacy).
- Status values: `open`, `in progress`, `done`, `local_complete / blocked_external`.
- Always link the roadmap stage the work belongs to.
