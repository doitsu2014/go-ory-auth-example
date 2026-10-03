# Intent Capture — Questions

Mode: YOLO (question budget 0). Each question records the recommended answer
as the `Recommendation`; the human may override any of them later.

## 1. What problem does this solve, and for whom?

- A. A reference project showing how to build first-party authentication for a web admin and a mobile app on the Ory stack, with a Go backend and PostgreSQL
- B. A production identity platform for many third-party apps (OAuth2 provider)
- C. A pure learning sandbox with no structure
- X. Other

[Answer]: A — Recommendation. Request names one admin website, one mobile app, and one Go identity service; no third-party clients mentioned.

## 2. What does success look like, in observable terms?

- A. An admin can sign in to the React admin; a mobile user can register and sign in from the Flutter app; both call the Go API with a validated session; all state lives in PostgreSQL; the whole stack starts locally with one command
- B. Only the docs exist
- X. Other

[Answer]: A — Recommendation. This intent's first deliverable is the `docs/` folder (architecture + design principles); the observable end-state is A.

## 3. What is explicitly out of scope?

- A. Third-party OAuth2 clients, multi-tenant SaaS, billing, production cloud provisioning (for the first iteration)
- B. Nothing is out of scope
- X. Other

[Answer]: A — Recommendation.

## 4. Hard deadlines, budget, or regulatory constraints?

- A. None stated; self-hosted open-source Ory (Apache-2.0) to avoid vendor cost
- B. Must use Ory Network (managed)
- X. Other

[Answer]: A — Recommendation. Ory Network stays a documented alternative.

## 5. What happens if we do nothing?

- A. The team builds auth ad hoc per app, duplicating password storage, session handling and security bugs
- X. Other

[Answer]: A — Recommendation.
