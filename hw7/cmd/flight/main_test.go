package main

import (
	"context"
	"testing"

	flightpb "github.com/nggurbanov/hse-microservices-course/hw7/gen/flightpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestMapFlightStatus(t *testing.T) {
	cases := map[string]flightpb.FlightStatus{
		"SCHEDULED": flightpb.FlightStatus_FLIGHT_STATUS_SCHEDULED,
		"departed":  flightpb.FlightStatus_FLIGHT_STATUS_DEPARTED,
		"CANCELLED": flightpb.FlightStatus_FLIGHT_STATUS_CANCELLED,
		"COMPLETED": flightpb.FlightStatus_FLIGHT_STATUS_COMPLETED,
		"bad":       flightpb.FlightStatus_FLIGHT_STATUS_UNSPECIFIED,
	}
	for input, want := range cases {
		if got := mapFlightStatus(input); got != want {
			t.Fatalf("mapFlightStatus(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestAuthInterceptor(t *testing.T) {
	interceptor := authInterceptor("secret")
	handler := func(ctx context.Context, req any) (any, error) { return "ok", nil }
	info := &grpc.UnaryServerInfo{FullMethod: "/flight.v1.FlightService/GetFlight"}

	_, err := interceptor(context.Background(), nil, info, handler)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("missing api key code = %v", status.Code(err))
	}

	badCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-api-key", "wrong"))
	_, err = interceptor(badCtx, nil, info, handler)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong api key code = %v", status.Code(err))
	}

	goodCtx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("x-api-key", "secret"))
	res, err := interceptor(goodCtx, nil, info, handler)
	if err != nil || res != "ok" {
		t.Fatalf("res=%v err=%v", res, err)
	}
}

func TestEnv(t *testing.T) {
	key := "FLIGHT_TEST_ENV"
	if got := env(key, "fallback"); got != "fallback" {
		t.Fatalf("env fallback = %q", got)
	}
	t.Setenv(key, "value")
	if got := env(key, "fallback"); got != "value" {
		t.Fatalf("env value = %q", got)
	}
}
