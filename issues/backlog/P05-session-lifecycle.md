# P05: Session lifecycle UX

Status: complete
Stage: P5
Requirements: 3, 11

## Goal
Session queue with wait estimate; session_summary; disconnect timing.

## Checklist
- [x] At MAX_SESSIONS: queue + estimated wait (not only 429)
- [x] WS `session_summary` on idle/max/disconnect terminate
- [x] Frontend SessionSummary modal (commands, crashes, duration)
- [x] Disconnect detect within 5s
- [x] CloudWatch emitter hooks prepared

## Exit criteria
- [x] Summary UI on forced terminate; queue at cap=1 under load
