COMPOSE := docker compose --env-file deploy/compose/.env -f deploy/compose/docker-compose.yml

.PHONY: env infra-up up down reset logs health seed smoke bao-init bao-token kek-rotate keys-rewrap

env: ## Create deploy/compose/.env from the example (local values only)
	@test -f deploy/compose/.env || cp deploy/compose/.env.example deploy/compose/.env

infra-up: env ## Start Postgres, Kratos, Keto, Mailpit, OpenBao (initialised + unsealed)
	$(COMPOSE) up -d --wait postgres mailpit kratos keto openbao
	$(COMPOSE) run --rm -T openbao-init

up: env ## Start the full stack including identity-service
	$(COMPOSE) --profile app up -d --build --wait

down: ## Stop the stack (keeps data)
	$(COMPOSE) --profile app down

reset: ## Stop the stack and delete all data
	$(COMPOSE) --profile app down -v

logs:
	$(COMPOSE) --profile app logs -f

health: ## Check component readiness
	@curl -fsS http://localhost:4433/health/ready >/dev/null && echo "kratos public: ok"
	@curl -fsS http://127.0.0.1:4434/admin/health/ready >/dev/null && echo "kratos admin: ok"
	@curl -fsS http://127.0.0.1:4466/health/ready >/dev/null && echo "keto read: ok"
	@curl -fsS http://127.0.0.1:4467/health/ready >/dev/null && echo "keto write: ok"
	@curl -fsS http://127.0.0.1:9090/readyz >/dev/null && echo "identity-service: ok" || echo "identity-service: not running"

seed: ## Create the first super_admin (prints a one-time recovery link)
	$(COMPOSE) --profile app exec identity-service /identity-service admin bootstrap --email admin@example.local

bao-init: ## Re-run OpenBao init/unseal (after `docker compose restart openbao`)
	$(COMPOSE) run --rm -T openbao-init

bao-token: ## Copy the identity-service OpenBao token for host-run tests (git-ignored)
	@mkdir -p deploy/compose/.local
	@$(COMPOSE) run --rm -T --entrypoint cat openbao-init /bao-app/identity-service.token > deploy/compose/.local/identity-service.token
	@chmod 600 deploy/compose/.local/identity-service.token && echo "deploy/compose/.local/identity-service.token"

kek-rotate: ## Rotate the PII key-encryption key (operator token, short-lived)
	$(COMPOSE) run --rm -T openbao-init rotate

keys-rewrap: ## Re-wrap every customer data key with the newest KEK version
	$(COMPOSE) --profile app exec identity-service /identity-service keys rewrap

smoke: ## End-to-end smoke test against the running stack
	node scripts/smoke.mjs
