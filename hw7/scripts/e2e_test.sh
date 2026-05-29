#!/usr/bin/env bash
set -euo pipefail

BASE_URL=${BASE_URL:-http://localhost:8080}
FLIGHT_ID=${FLIGHT_ID:-11111111-1111-1111-1111-111111111111}
COMPOSE=${COMPOSE:-docker compose}

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
"$SCRIPT_DIR/wait_for_system.sh"

before=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT available_seats FROM flights WHERE id='$FLIGHT_ID'")

resp=$(curl -fsS -X POST "$BASE_URL/bookings" \
  -H 'Content-Type: application/json' \
  -d "{\"passenger_name\":\"E2E User\",\"passenger_email\":\"e2e@example.com\",\"flight_id\":\"$FLIGHT_ID\",\"seat_count\":2}")

booking_id=$(python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["status"]=="CONFIRMED"; assert d["seat_count"]==2; print(d["id"])' <<<"$resp")

after_create=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT available_seats FROM flights WHERE id='$FLIGHT_ID'")
[ "$after_create" -eq $((before - 2)) ]

booking_row=$($COMPOSE exec -T booking-db psql -U booking -d booking -Atc "SELECT status || ':' || seat_count FROM bookings WHERE id='$booking_id'")
reservation_row=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT status || ':' || seat_count FROM seat_reservations WHERE booking_id='$booking_id'")
[ "$booking_row" = "CONFIRMED:2" ]
[ "$reservation_row" = "ACTIVE:2" ]

status=$(curl -sS -o /tmp/hw7_cancel_response.txt -w '%{http_code}' -X POST "$BASE_URL/bookings/$booking_id/cancel")
[ "$status" = "204" ]

after_cancel=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT available_seats FROM flights WHERE id='$FLIGHT_ID'")
[ "$after_cancel" -eq "$before" ]

booking_status=$($COMPOSE exec -T booking-db psql -U booking -d booking -Atc "SELECT status FROM bookings WHERE id='$booking_id'")
reservation_status=$($COMPOSE exec -T flight-db psql -U flight -d flight -Atc "SELECT status FROM seat_reservations WHERE booking_id='$booking_id'")
[ "$booking_status" = "CANCELLED" ]
[ "$reservation_status" = "RELEASED" ]

echo "e2e test passed"
