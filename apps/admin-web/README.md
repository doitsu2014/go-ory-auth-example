# admin-web

React admin console (unit U5). Talks to Kratos public with **browser flows**
and to identity-service `/admin/v1/*` with the Kratos session cookie.
Design: `docs/principles/03-admin-web-react.md`, `docs/architecture/03-auth-flows.md`.

## Commands

```bash
cp .env.example .env   # VITE_KRATOS_PUBLIC_URL, VITE_API_URL (public, non-secret)
pnpm install
pnpm dev               # http://localhost:5173 (strictPort; Kratos ui_urls + CORS expect it)
pnpm lint              # eslint (typescript-eslint strict, react-hooks, jsx-a11y strict) + prettier --check
pnpm typecheck         # tsc --noEmit
pnpm test --run        # vitest + React Testing Library + MSW
pnpm build             # dist/
pnpm generate:api      # regenerate src/api/schema.d.ts from api/openapi/identity-service.v1.yaml
pnpm e2e               # Playwright against the compose stack (needs `playwright install chromium`)
```

## Choices

| Concern   | Choice                                                              | Why                                                                                                                                                                                                                                                                                                                                                                       |
| --------- | ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Kratos UI | Own generic renderer of `ui.nodes` (`src/auth/ui`)                  | Full control of CSP (scripts ignored, QR only as `<img src="data:image/…">`), i18n by message id, and the §3.9 response handling; no dependency on Ory Elements' styling/runtime. Renders inputs (hidden `csrf_token` included), submit buttons with `name`/`value`, `img`, `text` (TOTP secret, lookup codes), anchors, node + flow messages. One form per method group. |
| UI kit    | Tailwind CSS v4 + small own components (`src/shared/ui`)            | Tiny, accessible, CSP-friendly (static CSS file, no runtime CSS-in-JS).                                                                                                                                                                                                                                                                                                   |
| Forms     | React Hook Form + Zod for non-Kratos forms (invite, disable reason) | Per principles. Kratos forms are uncontrolled and validated by Kratos.                                                                                                                                                                                                                                                                                                    |
| API       | `openapi-typescript` types + `openapi-fetch` (`src/api`)            | Contract-first (ADR-0010). Errors become `ApiError` with the problem `code`.                                                                                                                                                                                                                                                                                              |
| i18n      | `react-i18next`, `vi` default + `en`, switcher in the header        | Kratos messages rendered by id (`src/i18n/kratos.vi.ts`, ids from Kratos v26.2.0 `text/id.go`), fallback to Kratos `text`.                                                                                                                                                                                                                                                |

## Auth behaviour

- `RequireAdmin` loader (`src/auth/requireAdmin.ts`) asks `GET /admin/v1/me`:
  401 → `/login?return_to=…`, 403 `aal2_required` → `/login?aal=aal2&return_to=…`,
  403 `mfa_enrollment_required` → `/settings?enroll=totp`, 403 `not_admin`/`forbidden` → `/no-access`,
  200 → `{roles, permissions}` cached for (cosmetic) UI gating.
- The same mapping runs for every API error (QueryCache/MutationCache); other errors are toasts with the problem `code` → i18n.
- Kratos responses (`src/auth/kratosErrors.ts`, `useKratosFlow.ts`): 400+flow re-render, 403 `session_aal2_required` → aal2 login,
  403 `session_refresh_required` → `?refresh=true` login, 403 CSRF / 404 / 410 → new flow, 422 → follow `redirect_browser_to`
  (only same-origin or Kratos origin), 401 → login.
- `return_to` accepts relative in-app paths only (`src/auth/returnTo.ts`).
- Logout: `createBrowserLogoutFlow()` → `updateLogoutFlow(token)` → `queryClient.clear()`. 401 counts as signed out; any other failure keeps the session state and shows an error toast (FR-13).
- Flow errors show text by Kratos `error.id` (fallback: `reason`) with a "Try again" that creates a fresh flow with the same parameters. A Kratos redirect to its login-init endpoint (e.g. 422 to the aal2 step) is mapped to `/login?aal=aal2&return_to=…`.
- Production builds emit no source maps.
- No registration route or link; `/registration` (Kratos' `ui_url`) says registration is not available.

## Security headers (production)

CSP must be an **HTTP header** (`frame-ancestors` is ignored in `<meta>`). `public/_headers`
(Netlify / Cloudflare Pages format) ships with the build:

```
Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self' https://auth.example.com https://api.example.com; frame-ancestors 'none'; object-src 'none'; base-uri 'none'; form-action 'self' https://auth.example.com
```

Replace the two origins per environment. `img-src data:` is needed for the TOTP QR code.
Other hosts (nginx, CloudFront) must send the same headers. `public/_redirects` provides the SPA fallback.

## Security rules enforced by lint

`no-restricted-syntax` forbids `dangerouslySetInnerHTML` and `localStorage`/`sessionStorage`.
