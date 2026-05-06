#!/usr/bin/env sh
set -eu

state_file="${1:-.deploy/previous.env}"
compose_file="${COMPOSE_FILE:-docker-compose.prod.yaml}"
env_file="${ENV_FILE:-.env.prod}"

if [ ! -f "$state_file" ]; then
  echo "rollback: state file not found: $state_file" >&2
  exit 1
fi

# shellcheck disable=SC1090
. "$state_file"

: "${API_IMAGE:?API_IMAGE missing in rollback state}"
: "${WORKER_IMAGE:?WORKER_IMAGE missing in rollback state}"
: "${WEB_IMAGE:?WEB_IMAGE missing in rollback state}"

export API_IMAGE WORKER_IMAGE WEB_IMAGE

docker compose --env-file "$env_file" -f "$compose_file" pull api worker web
docker compose --env-file "$env_file" -f "$compose_file" up -d --no-build api worker web nginx
timeout 60 sh -c 'until docker exec repricer_api wget -qO- http://localhost:8080/healthz > /dev/null 2>&1; do sleep 2; done'

cp "$state_file" .deploy/current.env
echo "rollback: restored images from $state_file"
