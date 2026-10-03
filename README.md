# go-ory-auth-example

First-party identity platform on the Ory stack: Ory Kratos + Keto + Hydra, a Go
identity service, PostgreSQL, OpenBao (PII keys), a React admin website and a
Flutter mobile app.

Design docs: [docs/README.md](docs/README.md).

## Run it locally

Only **Docker** (with Compose v2) is required.

```bash
./dev up
```

This one command:

1. creates `deploy/compose/.env` from `.env.example` (local-only values) if missing;
2. builds and starts every container: Postgres, Kratos, Keto, Hydra, OpenBao
   (initialised and unsealed), Mailpit, identity-service and the admin web;
3. waits until every service is healthy;
4. creates the first `super_admin` (`admin@example.local`, override with
   `ADMIN_EMAIL=...`) and prints a one-time recovery link;
5. moves customer names that are still in Kratos into encrypted personal
   info (`pii migrate-kratos-names`, idempotent);
6. seeds 5 demo customers (`customer01..05@example.local`, password
   `SEED_CUSTOMER_PASSWORD` in `.env`) with encrypted name, phone, date of
   birth, address and national id, through the real registration flow (needs Node >= 22;
   skipped with a hint otherwise);
7. prints the URLs.

Then open the recovery link, set a password and enrol TOTP (MFA is mandatory
for admins). All emails land in Mailpit at http://localhost:8025.

| What | URL |
| --- | --- |
| Admin web | http://localhost:5173 |
| identity-service API | http://localhost:8080 |
| Kratos public | http://localhost:4433 |
| Hydra public (M2M tokens) | http://localhost:4444 |
| Mailpit | http://localhost:8025 |

Other commands (`./dev help`):

| Command | Does |
| --- | --- |
| `./dev status` | container state and health probes |
| `./dev logs [service]` | follow logs |
| `./dev seed EMAIL` | create another super_admin |
| `./dev seed-customers` | 5 demo customers with encrypted personal info (Node >= 22) |
| `./dev smoke` | end-to-end smoke test (Node >= 22) |
| `./dev mobile [android\|ios]` | run the Flutter app against the stack (Flutter SDK) |
| `./dev web` | admin web with Vite hot reload instead of the container (pnpm) |
| `./dev down` | stop, keep data |
| `./dev reset` | stop and delete all data (asks first) |

`make dev` is the same as `./dev up`. The finer-grained `make` targets
(`infra-up`, `bao-init`, `kek-rotate`, `keys-rewrap`, ...) are still there.

Configuration: all local secrets live in `deploy/compose/.env` (git-ignored).
The admin web's public URLs are build args of the `admin-web` service in
`deploy/compose/docker-compose.yml`. The mobile app reads `apps/mobile/env/local*.json`.
This setup is for local development only; production topology is in
[docs/architecture/05-deployment.md](docs/architecture/05-deployment.md).
