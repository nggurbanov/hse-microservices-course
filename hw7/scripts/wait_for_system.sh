#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
FLIGHT_METRICS_URL=${FLIGHT_METRICS_URL:-http://localhost:9090}
PROM_URL=${PROM_URL:-http://localhost:9091}

echo "waiting for booking-service"
for _ in $(seq 1 60); do
  if curl -fsS "$BASE_URL/healthz" >/dev/null; then break; fi
  sleep 2
done
curl -fsS "$BASE_URL/healthz" >/dev/null

echo "waiting for flight-service metrics"
for _ in $(seq 1 60); do
  if curl -fsS "$FLIGHT_METRICS_URL/healthz" >/dev/null; then break; fi
  sleep 2
done
curl -fsS "$FLIGHT_METRICS_URL/healthz" >/dev/null

echo "waiting for prometheus"
for _ in $(seq 1 60); do
  if curl -fsS "$PROM_URL/-/ready" >/dev/null; then break; fi
  sleep 2
done
curl -fsS "$PROM_URL/-/ready" >/dev/null
