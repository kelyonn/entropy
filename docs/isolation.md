# Sandbox isolation

Entropy runs arbitrary root commands from anonymous users. Treat every sandbox
as hostile.

## Layers

| Layer | Setting | Where |
|---|---|---|
| Kernel boundary | gVisor (`runsc`) when available | `services/airlock/container/isolation.go` |
| Capabilities | drop ALL, add CHOWN, SETUID, SETGID only | same |
| Privilege escalation | `no-new-privileges` | same |
| Resources | 256 MB memory (no swap), 0.5 CPU, 100 PIDs, nofile 1024/2048, 512 MB max file | orchestrator + isolation |
| Network | `none` (lab stacks use a private internal network, no egress) | orchestrator |
| Filesystems | `/tmp` and `/var/log` are noexec, nosuid tmpfs | orchestrator |
| /proc and /sys | Docker default masked and read-only paths | left unset on purpose |
| WebSocket | Origin allowlist (`ALLOWED_ORIGINS`) plus same host | `services/session-manager/handlers/origin.go` |

## gVisor (required before public exposure)

Without gVisor every sandbox shares the host kernel, and one kernel bug is a
host compromise. On the VM:

1. Install runsc from the gVisor release channel (https://gvisor.dev/docs/user_guide/install/).
2. Register it: `sudo runsc install && sudo systemctl restart docker`.
3. Confirm: `docker info --format '{{json .Runtimes}}'` lists `runsc`.
4. Set `SANDBOX_RUNTIME=runsc` in `.env` so Airlock refuses to start sandboxes without it.
5. Verify the labs under gVisor:
   `cd services/session-manager && ENTROPY_LAB_SMOKE=1 ENTROPY_LAB_RUNTIME=runsc go test ./labs/ -run Docker -v`

`SANDBOX_RUNTIME` values: `auto` (default; runsc if registered, otherwise runc
with a warning), `runsc` or another registered runtime (fail closed), `runc`
(development only).

## Known residual risk

- Airlock and the eBPF monitor mount the Docker socket, which is equivalent to
  root on the host. Neither is exposed outside the Compose network. A socket
  proxy that allows only the container, exec, and image endpoints Airlock uses
  is the next hardening step.
- gVisor was not available in the environment where this was written; the
  runtime selection is unit tested and the labs pass under runc with the
  reduced capability set. Run step 5 on the VM.
