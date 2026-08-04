# Close-out paste-back (Phase A)

The public free-stack close-out cannot proceed until you paste the items below.
Do them in order. After each block, paste the command output (redact secrets:
never paste the full tunnel token; confirm you have it with `token_set=yes`).

## 1. Linux VM with Docker

On the VM:

```bash
uname -a
docker version --format '{{.Server.Version}}'
docker compose version
df -h /
free -h || true
```

Also provide how the agent (or you) will reach it, for example:

```text
ssh_user=ubuntu
ssh_host=203.0.113.10
# or: I will run commands myself and paste outputs
```

## 2. Repo on the VM

```bash
cd ~ # or your preferred path
git clone <YOUR_REPO_URL> entropy || true
cd entropy
git checkout kalyan_dev || git branch --show-current
git rev-parse --short HEAD
```

## 3. Cloudflare named tunnel

In Cloudflare Zero Trust → Networks → Tunnels:

1. Create a tunnel named `entropy`.
2. Copy the install token into the VM `.env` as `CLOUDFLARE_TUNNEL_TOKEN=...`
   (do not paste the token into chat).
3. Confirm DNS for your domain is on Cloudflare.

Paste:

```text
cloudflare_domain=example.com
app_hostname=app.example.com
auth_hostname=auth.example.com
tunnel_token_set=yes
```

## 4. Ready signal

When 1–3 are done, reply with:

```text
closeout_ready=yes
```

The agent will then continue: Compose boot, tunnel profile, Keycloak redirects,
live acceptance, and STATUS close-out.
