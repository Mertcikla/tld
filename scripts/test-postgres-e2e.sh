#!/usr/bin/env bash
set -euo pipefail

container="${TLD_POSTGRES_CONTAINER:-tld-postgres-test}"
port="${TLD_POSTGRES_PORT:-55432}"
user="${TLD_POSTGRES_USER:-tld}"
password="${TLD_POSTGRES_PASSWORD:-tld}"
database="${TLD_POSTGRES_DB:-tld}"
image="${TLD_POSTGRES_IMAGE:-pgvector/pgvector:pg16}"

if ! docker ps --format '{{.Names}}' | grep -qx "$container"; then
  docker rm -f "$container" >/dev/null 2>&1 || true
  docker run -d \
    --name "$container" \
    -e POSTGRES_USER="$user" \
    -e POSTGRES_PASSWORD="$password" \
    -e POSTGRES_DB="$database" \
    -p "$port:5432" \
    "$image" >/dev/null
fi

for _ in $(seq 1 60); do
  if docker exec "$container" pg_isready -U "$user" -d "$database" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! docker exec "$container" pg_isready -U "$user" -d "$database" >/dev/null 2>&1; then
  docker logs "$container" >&2 || true
  echo "postgres container did not become ready" >&2
  exit 1
fi

export TLD_TEST_POSTGRES_URL="${TLD_TEST_POSTGRES_URL:-postgres://${user}:${password}@localhost:${port}/${database}?sslmode=disable}"
go test -tags integration ./pkg/dbrepo -count=1 -v
go test -tags integration ./tests/postgres -count=1 -v
