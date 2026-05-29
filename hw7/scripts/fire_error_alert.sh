#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
end=$((SECONDS + 45))
while [ "$SECONDS" -lt "$end" ]; do
  curl -sS -o /dev/null -X POST "$BASE_URL/bookings" -H 'Content-Type: application/json' -d '{bad json' || true
  sleep 1
done

echo "sent bad requests for 45 seconds; open http://localhost:9091/alerts or http://localhost:9093"
