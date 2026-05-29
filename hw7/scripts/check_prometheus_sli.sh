#!/usr/bin/env bash
set -euo pipefail

PROM_URL=${PROM_URL:-http://localhost:9091}
mkdir -p artifacts

query() {
  local q="$1"
  curl -fsS --get "$PROM_URL/api/v1/query" --data-urlencode "query=$q"
}

value() {
  python3 -c 'import json,sys; d=json.load(sys.stdin); r=d["data"]["result"]; print(r[0]["value"][1] if r else "0")'
}

sleep 10

error_json=$(query 'sum(rate(http_requests_total{endpoint="/bookings",status=~"5.."}[15m])) / clamp_min(sum(rate(http_requests_total{endpoint="/bookings"}[15m])), 1)')
latency_json=$(query 'histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{endpoint="/bookings"}[15m])))')
avail_json=$(query 'sum(rate(http_requests_total{endpoint="/bookings",status!~"5.."}[15m])) / clamp_min(sum(rate(http_requests_total{endpoint="/bookings"}[15m])), 1)')
grpc_json=$(query 'sum(rate(grpc_requests_total{status!~"Internal|Unavailable|DeadlineExceeded|Unknown"}[15m])) / clamp_min(sum(rate(grpc_requests_total[15m])), 1)')

printf '%s\n' "$error_json" > artifacts/prom_error_rate.json
printf '%s\n' "$latency_json" > artifacts/prom_p95_latency.json
printf '%s\n' "$avail_json" > artifacts/prom_availability.json
printf '%s\n' "$grpc_json" > artifacts/prom_grpc_success.json

error_rate=$(value <<<"$error_json")
p95=$(value <<<"$latency_json")
availability=$(value <<<"$avail_json")
grpc_success=$(value <<<"$grpc_json")

python3 - <<PY
error_rate=float('$error_rate')
p95=float('$p95')
availability=float('$availability')
grpc_success=float('$grpc_success')
print(f'error_rate={error_rate:.6f}')
print(f'p95_latency_seconds={p95:.6f}')
print(f'availability={availability:.6f}')
print(f'grpc_success={grpc_success:.6f}')
assert error_rate < 0.01, f'error_rate too high: {error_rate}'
assert p95 < 1.0, f'p95 too high: {p95}'
assert availability >= 0.99, f'availability too low: {availability}'
assert grpc_success >= 0.99, f'grpc success too low: {grpc_success}'
PY
