# Product memory

## What Entropy is
An AI-powered chaos-engineering sandbox. Developers safely run dangerous system
commands inside isolated containers, watch the consequences through a live
terminal and a system-health visualization, and receive AI root-cause
explanations when things fail. It turns catastrophic errors into learning.

## Problem it solves
- Skill gap: shortage of systems and reliability engineers.
- Fear factor: junior developers avoid risky operations because mistakes are
  costly in production.
- Tool gap: no safe "flight simulator" for practicing infrastructure failures.

## Target users and personas
- Priya, CS student (21): wants to understand syscalls and kernel behavior;
  afraid to experiment on her laptop. Success: safely run `rm -rf /`, observe
  the failure, understand why.
- Rahul, junior backend engineer (24): transitioning to DevOps; no production
  access. Success: practice OOM kills and resource limits.
- Maya, aspiring SRE (26): preparing for interviews; cannot afford cloud chaos
  practice. Success: complete guided scenarios and save interesting states.

## Core experience
1. Start a guest session or sign in through Keycloak (OIDC).
2. Run commands in a real terminal.
3. See health change in the visualization.
4. On failure, read an AI explanation: root cause, system impact, remediation
   (default LLM: Ollama).
5. Run guided scenarios and save or restore snapshots.

## Success metrics (from requirements)
- Engagement: average session over 15 minutes; 20+ commands per session.
- Learning: 80% of failures trigger an explanation view.
- Reliability: 99% session creation success; 95% explanation success.
- Cost: free-stack self-host; keep per-session resource limits tight.

## Constraints that shape the product
- Container limits: 0.5 CPU, 256 MB RAM, max 100 concurrent sessions.
- Session lifetime: 5 minute idle timeout, 1 hour hard cap.
- AI: max 500 tokens per explanation, 10 explanations per session.
- Delivery: free Linux VM + Docker Compose; Cloudflare Tunnel for public HTTPS.
