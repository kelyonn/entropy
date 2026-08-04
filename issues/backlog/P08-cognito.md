# P08: Keycloak auth

Status: local_complete / blocked_free_stack (public hostname redirects)
Stage: P8
Requirements: 1, 11

## Goal
Real Keycloak (OIDC) + frontend auth adapter + JWT middleware; guest retained.

## Checklist
- [x] Keycloak realm + client configuration (OIDC) — `keycloak/entropy-realm.json`
- [x] Frontend auth adapter wired to Keycloak (`keycloak-js`)
- [x] Session Manager JWT verification via shared `pkg/auth` OIDC validator
- [x] Guest path unchanged; history behavior remains consistent
- [x] Auth failure shows descriptive error on login UI

## Exit criteria
- [x] Guest + OIDC code path against Compose Keycloak (local)
- [ ] Email sign-in against Keycloak with public tunnel hostnames (operator)
