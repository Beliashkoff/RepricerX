# Production Deploy And Rollback

Production deploys are gated by the `CI` workflow. CI runs tests and lint, then publishes immutable GHCR images tagged with the commit SHA. The deploy workflow runs only after CI completes successfully for `main`.

Runtime containers must not build images on the production host. `docker-compose.prod.yaml` requires:

- `API_IMAGE=ghcr.io/<owner>/repricerx/api:<git-sha>`
- `WORKER_IMAGE=ghcr.io/<owner>/repricerx/worker:<git-sha>`
- `WEB_IMAGE=ghcr.io/<owner>/repricerx/web:<git-sha>`

Migrations and legacy credentials encryption are separate one-shot jobs:

```sh
docker compose --env-file .env.prod -f docker-compose.prod.yaml up --no-build --no-deps --exit-code-from migrate migrate
docker compose --env-file .env.prod -f docker-compose.prod.yaml up --no-build --no-deps --exit-code-from credbackfill credbackfill
```

Only after both commands exit successfully should `api`, `worker`, `web`, and `nginx` be started. The `credbackfill` job is idempotent: it skips already encrypted shop credentials and encrypts plaintext rows produced by legacy marketplace migrations. The API binary does not apply migrations on startup.

Rollback uses the previous immutable images saved by deploy:

```sh
scripts/prod/rollback.sh
```

This rolls back application containers only. Database rollback is intentionally not automatic. Production migrations must be backward-compatible with the previous application version. If a database rollback is unavoidable, take a fresh backup, stop `api` and `worker`, verify the exact migration version to reverse, run the migration rollback manually, and then start the selected immutable image set.
