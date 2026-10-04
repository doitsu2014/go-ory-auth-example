# Repository Structure

```
go-ory-auth-example/
├── api/
│   └── openapi/
│       └── identity-service.v1.yaml      # contract (source of truth, ADR-0010)
├── services/
│   └── identity-service/                 # Go (see principles/02-backend-go.md)
├── apps/
│   ├── admin-web/                        # React + Vite + TS
│   └── mobile/                           # Flutter
├── deploy/
│   ├── compose/
│   │   ├── docker-compose.yml            # full local stack
│   │   └── .env.example                  # local-only values; .env is git-ignored
│   ├── postgres/
│   │   └── init/01-databases.sh          # kratos, keto, identity DBs + roles
│   └── ory/
│       ├── kratos/
│       │   ├── kratos.yml.tmpl            # rendered at deploy (webhook key injected)
│       │   ├── identity-schemas/
│       │   │   ├── customer.v2.json (+ customer.v2.transition.json)
│       │   │   └── admin.v1.json
│       │   ├── webhooks/after-{registration,login}.jsonnet
│       │   └── courier-templates/        # vi/en email templates
│       └── keto/
│           ├── keto.yml
│           └── namespaces.keto.ts        # OPL permission model
├── docs/                                 # this documentation
├── scripts/                              # smoke.mjs (end-to-end smoke test)
├── .github/workflows/                    # CI per app (path filters)
├── Makefile                              # up, down, seed, generate, lint, test, e2e
└── README.md
```

Conventions

- Each app is self-contained (own `Makefile`/`package.json`/`pubspec.yaml`)
  and runnable on its own against the compose stack.
- Generated code lives next to its consumer and is committed.
- Infra config for Ory lives in `deploy/ory`, never inside an app.
- Root `Makefile` targets call into app-level targets so CI and developers run
  the same commands.
