# S07: AI streaming explanations

Status: done
Stage: S7

## Goal
Stream Bedrock explanations with rate limit, cache, and fallbacks.

## Checklist
- [ ] InvokeModelWithResponseStream in backend/ai/bedrock.go
- [ ] Redis cache TTL 24h
- [ ] Rate limit 10 explanations per session
- [ ] Generic exit-code fallbacks (no emoji)
- [ ] Frontend streams tokens into Markdown AI panel

## Exit criteria
- [ ] First token streams; full text renders Markdown
- [ ] Forced Bedrock failure shows usable fallback
