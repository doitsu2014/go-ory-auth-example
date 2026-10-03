# 0003. Browser flows for admin web, native API flows for mobile

- Status: Proposed
- Date: 2026-10-03

## Context

Kratos offers two flavours of every self-service flow. Browser flows use
cookies + CSRF tokens; API flows return JSON and a session token. Ory docs warn
never to use API flows in browser apps (login-CSRF and related attacks).

## Decision

- Admin web (SPA) uses **browser flows** via AJAX (`Accept: application/json`,
  `credentials: include`). Kratos public and the SPA share a parent domain.
- Mobile uses **API flows** and stores the session token in secure storage.

## Alternatives considered

- **API flows in the SPA + token in memory/localStorage** — explicitly
  discouraged by Ory; XSS-exposed tokens.
- **BFF for the admin web** (server holds the session) — stronger isolation of
  the cookie, but an extra deployable; Kratos' HttpOnly cookie already gives
  most of the benefit.
- **WebView with browser flows on mobile** — poor UX, cookie jar quirks.

## Consequences

- Cookie domain / CORS must be configured correctly (risk R-01, spike S2).
- Two client integration styles to document and test.

## Revisit when

The admin web must run on a domain unrelated to Kratos → use Kratos behind the
admin host via path prefix, or introduce a BFF.
