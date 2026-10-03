# Admin Web Guidelines (React)

## 1. Stack

| Concern | Choice |
| --- | --- |
| Build | Vite + TypeScript (strict) |
| Routing | React Router (data routers, loaders for auth guards) |
| Server state | TanStack Query |
| Ory | `@ory/client-fetch` (`FrontendApi`, `credentials: "include"`); `@ory/elements-react` for login / settings / recovery forms (themeable) |
| API client | `openapi-typescript` + `openapi-fetch` generated from `api/openapi/identity-service.v1.yaml` |
| UI kit | One component library (e.g. shadcn/ui + Tailwind, or Mantine) — pick once, record in ADR |
| Forms | React Hook Form + Zod (for non-Kratos forms) |
| i18n | `react-i18next` (vi, en) |
| Tests | Vitest + React Testing Library; Playwright E2E; MSW for API mocks |
| Lint | ESLint (typescript-eslint, react-hooks, jsx-a11y) + Prettier |
| Package manager | pnpm |

## 2. Layout

```
apps/admin-web/
├── src/
│   ├── app/            # router, providers (QueryClient, i18n, OryProvider), layout
│   ├── auth/           # ory.ts (FrontendApi), useSession, RequireAdmin guard, step-up helpers
│   ├── features/
│   │   ├── login/      # Login page (Ory Elements), AAL2 step-up
│   │   ├── settings/   # password, TOTP, backup codes
│   │   ├── customers/  # list, detail, disable/enable, revoke sessions
│   │   ├── admins/     # list, invite, change role
│   │   └── audit/      # audit events table
│   ├── api/            # generated schema.d.ts + typed client wrapper
│   ├── shared/         # ui components, hooks, utils
│   └── i18n/
├── e2e/                # Playwright specs
└── vite.config.ts
```

Feature folders own their routes, components, queries and tests.

## 3. Auth rules

1. **Browser flows only.** Initialise with `createBrowserLoginFlow()` etc.; never call `*/api` flows.
2. **No registration route.** The app does not link to or render registration.
3. **Session = HttpOnly cookie.** Never read, copy or store it; every request uses `credentials: "include"`.
4. **Guard by asking the API.** `RequireAdmin` loader calls `GET /admin/v1/me`:
   - `401` → redirect `/login?return_to=…`
   - `403 aal2_required` → `/login?aal=aal2&return_to=…`
   - `403 mfa_enrollment_required` → `/settings?enroll=totp`
   - `403 not_admin` → show "No access" + logout
   - `200` → store `{roles, permissions}` in query cache for UI gating.
5. **UI gating is cosmetic.** Hide buttons the user can't use, but always handle `403` from the API.
6. **Return URLs** must be relative paths or in Kratos `allowed_return_urls`.
7. **Logout** via `createBrowserLogoutFlow()` → `logout_url`; then clear the query cache.
8. Render Kratos messages by `id` through i18n; fallback to `text`.

## 4. Coding rules

- Strict TypeScript (`strict`, `noUncheckedIndexedAccess`); no `any` in feature code.
- Server state only in TanStack Query; local UI state in components; no global store unless justified.
- Query keys centralised per feature (`customerKeys.list(params)`).
- Mutations invalidate exactly the affected keys and show toast on success/failure (problem `code` → i18n).
- Tables use server-side cursor pagination from the API (`next_page_token`).
- Accessibility: every interactive element keyboard-reachable, labelled; `jsx-a11y` errors block CI.
- No `dangerouslySetInnerHTML`; CSP-compatible (no inline scripts).
- Env: only `VITE_KRATOS_PUBLIC_URL`, `VITE_API_URL`; no secrets ever (bundle is public).

## 5. Testing

- Unit/component: guards' redirect matrix, forms, table states (MSW).
- E2E (Playwright, compose stack): invite admin via bootstrap → recovery link (Mailpit API) → set password → enrol TOTP (generate code with `otplib`) → step-up → disable a customer → audit entry visible.
