#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
FLIGHT_ID=${FLIGHT_ID:-11111111-1111-1111-1111-111111111111}
COMPOSE=${COMPOSE:-docker compose}

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
"$SCRIPT_DIR/wait_for_system.sh"

before=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT available_seats FROM flights WHERE id='$FLIGHT_ID'")

body=$(cat <<JSON
{"passenger_name":"Integration User","passenger_email":"integration@example.com","flight_id":"$FLIGHT_ID","seat_count":3}
JSON
)
resp=$(curl -fsS -X POST "$BASE_URL/bookings" -H 'Content-Type: application/json' -d "$body")
booking_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])' <<<"$resp")
[ -n "$booking_id" ]

after=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT available_seats FROM flights WHERE id='$FLIGHT_ID'")
if [ "$after" -ne $((before - 3)) ]; then
  echo "expected available seats $((before - 3)), got $after"
  exit 1
fi

booking_status=$($COMPOSE exec -T booking-db psql -U booking -d booking -Atc "SELECT status FROM bookings WHERE id='$booking_id'")
reservation_status=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT status FROM seat_reservations WHERE booking_id='$booking_id'")
[ "$booking_status" = "CONFIRMED" ]
[ "$reservation_status" = "ACTIVE" ]

status=$(curl -sS -o /tmp/hw7_integration_cancel_response.txt -w '%{http_code}' -X POST "$BASE_URL/bookings/$booking_id/cancel")
[ "$status" = "204" ]
restored=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT available_seats FROM flights WHERE id='$FLIGHT_ID'")
[ "$restored" -eq "$before" ]

count_before=$($COMPOSE exec -T booking-db psql -U booking -d booking -Atc "SELECT count(*) FROM bookings")
status=$(curl -sS -o /tmp/hw7_overbook_response.txt -w '%{http_code}' -X POST "$BASE_URL/bookings" \
  -H 'Content-Type: application/json' \
  -d "{\"passenger_name\":\"Too Many\",\"passenger_email\":\"bad@example.com\",\"flight_id\":\"$FLIGHT_ID\",\"seat_count\":200000}")
if [ "$status" != "409" ]; then
  echo "expected overbooking HTTP 409, got $status"
  cat /tmp/hw7_overbook_response.txt
  exit 1
fi
count_after=$($COMPOSE exec -T booking-db psql -U booking -d booking -Atc "SELECT count(*) FROM bookings")
[ "$count_before" = "$count_after" ]

echo "integration test passed"
