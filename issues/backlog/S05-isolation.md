# S05: Isolation hardening

Status: done
Stage: S5

## Checklist
- [x] NetworkMode none
- [x] CapDrop ALL + minimal CapAdd
- [x] no-new-privileges
- [x] PidsLimit 100
- [x] tmpfs /tmp and /var/log

## Verification
```
# inside sandbox (network none): curl should fail
# fork bomb should hit PID limit
```
