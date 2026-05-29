CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE bookings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    passenger_name VARCHAR(255) NOT NULL,
    passenger_email VARCHAR(255) NOT NULL,
    flight_id UUID NOT NULL,
    seat_count INTEGER NOT NULL CHECK (seat_count > 0),
    total_price NUMERIC(12,2) NOT NULL CHECK (total_price > 0),
    status VARCHAR(32) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
