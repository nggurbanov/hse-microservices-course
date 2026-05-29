#!/usr/bin/env bash
set -euo pipefail

mkdir -p artifacts
SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
"$SCRIPT_DIR/wait_for_system.sh"

docker compose run --rm \
  -e BASE_URL=http://booking-service:8080 \
  -e FLIGHT_ID=33333333-3333-3333-3333-333333333333 \
  -e VUS=${VUS:-10} \
  -e DURATION=${DURATION:-30s} \
  k6 run --summary-export /artifacts/k6-summary.json /scripts/booking.js | tee artifacts/k6.log
