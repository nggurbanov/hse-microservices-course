# HW3 Flight Booking

Сделал вариант на Go: `booking-service` отдаёт REST, `flight-service` поднимает gRPC и кэширует чтения в Redis.

Что есть:

- `SearchFlights`, `GetFlight`, `ReserveSeats`, `ReleaseReservation` в `proto/flight.proto`
- две отдельные PostgreSQL
- миграции через Flyway
- аутентификация gRPC по API key через metadata
- Redis cache-aside для `GetFlight` и `SearchFlights`
- retry в `booking-service` для `UNAVAILABLE` и `DEADLINE_EXCEEDED`
- транзакции и `SELECT ... FOR UPDATE` при резервировании/освобождении мест

Запуск:

```bash
cd hw3
docker-compose up --build
```

Примеры:

```bash
curl -X POST localhost:8080/bookings \
  -H 'Content-Type: application/json' \
  -d '{"passenger_name":"Ivan Petrov","passenger_email":"ivan@example.com","flight_id":"<uuid>", "seat_count":2}'
```

ER-диаграмма:

```mermaid
erDiagram
    FLIGHTS ||--o{ SEAT_RESERVATIONS : has
    FLIGHTS {
        uuid id PK
        string flight_number
        string airline
        string origin
        string destination
        timestamp departure_time
        timestamp arrival_time
        int total_seats
        int available_seats
        decimal price
        string status
    }
    SEAT_RESERVATIONS {
        uuid id PK
        uuid booking_id UK
        uuid flight_id FK
        int seat_count
        string status
    }
    BOOKINGS {
        uuid id PK
        string passenger_name
        string passenger_email
        uuid flight_id
        int seat_count
        decimal total_price
        string status
    }
```
