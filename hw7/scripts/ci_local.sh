#!/usr/bin/env bash
set -euo pipefail

go test ./...
docker compose build
docker compose up -d
./scripts/wait_for_system.sh
./scripts/integration_test.sh
./scripts/e2e_test.sh
./scripts/load_test.sh
./scripts/check_prometheus_sli.sh
