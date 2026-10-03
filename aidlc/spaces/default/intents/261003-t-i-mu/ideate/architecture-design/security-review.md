# Security Review — architecture-design

Reviewer: security-agent (advisory class; blocking findings applied before reporting).
Ory claims are checked against reviewer knowledge of Kratos v1.x and still
need confirming in spikes S2/S3.

## Blocking — fixed

| # | Finding | Fix applied in |
| --- | --- | --- |
| B1 | Kratos admin `POST /admin/recovery/code` returns link/code but does not email them; invites would never arrive or would leak through the API | 03-auth-flows §3.6, api doc (own mailer, link never in the response or logs), 02-components |
| B2 | `${VAR}` isn't expanded in Kratos YAML; env overrides have no `KRATOS_` prefix | 05-deployment §5.3 (`SECRETS_*`, config rendering, fail when unset), api webhook YAML, 06 §6.3 |
| B3 | whoami returns `403 session_aal2_required` under `highest_available`; treating it as 401 loops admins on the login page | 03 §3.4 diagram + notes, ADR-0006, 02-backend-go §4, 06 §6.3 |

## Advisory — applied

| # | Finding | Resolution |
| --- | --- | --- |
| A1 | `code` method could become an admin factor | `passwordless_enabled/mfa_enabled: false`; no `credentials.code` in admin schema |
| A2 | Parent-domain cookie reaches every sibling subdomain | Dedicated platform apex (accepted risk recorded); host-only `/.ory` layout documented as the alternative |
| A3 | Credentials not bound to plane | Cookie-only `/admin/v1`, bearer-only `/v1`; interrupting after-login webhook made required (ADR-0004) |
| A4 | Wrong claim that whoami extends sessions; 12 h cap didn't revoke | Extension path corrected; the cap revokes through the admin API; `persistent: false` |
| A5 | Role tuple built from input | Closed enum → relation mapping, target schema check, no self-change, last-super-admin guard |
| A6 | :8081 shared webhooks with probes | Ops port `:9090`; :8081 webhook-only |
| A7 | A stolen password could enrol an attacker's TOTP | Enrolment inside the invite session; 24 h deadline |
| A8 | CSP `frame-ancestors` ignored in a meta tag | CSP sent as an HTTP header |

No open blocking findings.
