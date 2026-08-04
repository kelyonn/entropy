# S03: Guest session lifecycle

Status: done
Stage: S3

## Checklist
- [x] Statuses CREATING|ACTIVE|IDLE|TERMINATING|TERMINATED
- [x] MAX_SESSIONS configurable
- [x] Idle 5m + max lifetime 1h sweeper
- [x] Disconnect grace cleanup

## Exit criteria
- [x] Manager + sweeper implemented
- [x] 429 when session limit reached
