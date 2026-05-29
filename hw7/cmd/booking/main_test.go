package main

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStatusToHTTP(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"not found", status.Error(codes.NotFound, "missing"), http.StatusNotFound},
		{"resource exhausted", status.Error(codes.ResourceExhausted, "full"), http.StatusConflict},
		{"unauthenticated", status.Error(codes.Unauthenticated, "bad key"), http.StatusUnauthorized},
		{"unknown", errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusToHTTP(tc.err); got != tc.want {
				t.Fatalf("statusToHTTP() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRetryCallRetriesTemporaryGrpcErrors(t *testing.T) {
	attempts := 0
	res, err := retryCall(func(ctx context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", status.Error(codes.Unavailable, "try later")
		}
		return "ok", nil
	}, context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res != "ok" || attempts != 3 {
		t.Fatalf("res=%q attempts=%d", res, attempts)
	}
}

func TestRetryCallDoesNotRetryBusinessError(t *testing.T) {
	attempts := 0
	_, err := retryCall(func(ctx context.Context) (string, error) {
		attempts++
		return "", status.Error(codes.NotFound, "missing")
	}, context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d, want 1", attempts)
	}
}

func TestEnv(t *testing.T) {
	key := "BOOKING_TEST_ENV"
	if got := env(key, "fallback"); got != "fallback" {
		t.Fatalf("env fallback = %q", got)
	}
	t.Setenv(key, "value")
	if got := env(key, "fallback"); got != "value" {
		t.Fatalf("env value = %q", got)
	}
}
