CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE flights (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flight_number VARCHAR(16) NOT NULL,
    airline VARCHAR(100) NOT NULL,
    origin CHAR(3) NOT NULL,
    destination CHAR(3) NOT NULL,
    departure_time TIMESTAMP NOT NULL,
    arrival_time TIMESTAMP NOT NULL,
    total_seats INTEGER NOT NULL CHECK (total_seats > 0),
    available_seats INTEGER NOT NULL CHECK (available_seats >= 0 AND available_seats <= total_seats),
    price NUMERIC(12,2) NOT NULL CHECK (price > 0),
    status VARCHAR(32) NOT NULL,
    UNIQUE (flight_number, departure_time)
);

CREATE TABLE seat_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    booking_id UUID NOT NULL UNIQUE,
    flight_id UUID NOT NULL REFERENCES flights(id),
    seat_count INTEGER NOT NULL CHECK (seat_count > 0),
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

INSERT INTO flights (id, flight_number, airline, origin, destination, departure_time, arrival_time, total_seats, available_seats, price, status)
VALUES
('11111111-1111-1111-1111-111111111111', 'SU1234', 'Aeroflot', 'VKO', 'LED', '2026-04-01 10:00:00', '2026-04-01 11:30:00', 120, 120, 4500.00, 'SCHEDULED'),
('22222222-2222-2222-2222-222222222222', 'S7101', 'S7', 'DME', 'KZN', '2026-04-01 12:00:00', '2026-04-01 13:45:00', 100, 100, 5200.00, 'SCHEDULED'),
('33333333-3333-3333-3333-333333333333', 'LOAD1', 'Demo Air', 'VKO', 'LED', '2026-04-01 14:00:00', '2026-04-01 15:30:00', 100000, 100000, 1000.00, 'SCHEDULED');
