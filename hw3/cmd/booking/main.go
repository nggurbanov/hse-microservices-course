package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	flightpb "github.com/nggurbanov/hse-microservices-course/hw3/gen/flightpb"
)

type createBookingRequest struct {
	PassengerName  string `json:"passenger_name"`
	PassengerEmail string `json:"passenger_email"`
	FlightID       string `json:"flight_id"`
	SeatCount      int32  `json:"seat_count"`
}

type bookingResponse struct {
	ID             string  `json:"id"`
	PassengerName  string  `json:"passenger_name"`
	PassengerEmail string  `json:"passenger_email"`
	FlightID       string  `json:"flight_id"`
	SeatCount      int32   `json:"seat_count"`
	TotalPrice     float64 `json:"total_price"`
	Status         string  `json:"status"`
}

func main() {
	db, err := pgxpool.New(context.Background(), os.Getenv("DB_URL"))
	if err != nil {
		log.Fatal(err)
	}
	conn, err := grpc.NewClient(env("FLIGHT_GRPC_ADDR", "localhost:50051"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal(err)
	}
	client := flightpb.NewFlightServiceClient(conn)

	http.HandleFunc("/bookings", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			createBooking(w, r, db, client)
			return
		}
		http.NotFound(w, r)
	})
	http.HandleFunc("/bookings/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/cancel") && r.Method == http.MethodPost {
			cancelBooking(w, r, db, client)
			return
		}
		http.NotFound(w, r)
	})
	log.Fatal(http.ListenAndServe(env("HTTP_ADDR", ":8080"), nil))
}

func createBooking(w http.ResponseWriter, r *http.Request, db *pgxpool.Pool, client flightpb.FlightServiceClient) {
	var req createBookingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	bookingID := uuid.NewString()
	ctx := authCtx(r.Context())
	flight, err := retryCall(func(ctx context.Context) (*flightpb.Flight, error) {
		return client.GetFlight(ctx, &flightpb.GetFlightRequest{FlightId: req.FlightID})
	}, ctx)
	if err != nil {
		http.Error(w, err.Error(), statusToHTTP(err))
		return
	}
	_, err = retryCall(func(ctx context.Context) (*flightpb.ReserveSeatsResponse, error) {
		return client.ReserveSeats(ctx, &flightpb.ReserveSeatsRequest{BookingId: bookingID, FlightId: req.FlightID, SeatCount: req.SeatCount})
	}, ctx)
	if err != nil {
		http.Error(w, err.Error(), statusToHTTP(err))
		return
	}
	totalPrice := float64(req.SeatCount) * flight.Price
	_, err = db.Exec(r.Context(), `INSERT INTO bookings (id, passenger_name, passenger_email, flight_id, seat_count, total_price, status) VALUES ($1,$2,$3,$4,$5,$6,'CONFIRMED')`,
		bookingID, req.PassengerName, req.PassengerEmail, req.FlightID, req.SeatCount, totalPrice)
	if err != nil {
		_, _ = retryCall(func(ctx context.Context) (*flightpb.ReleaseReservationResponse, error) {
			return client.ReleaseReservation(ctx, &flightpb.ReleaseReservationRequest{BookingId: bookingID})
		}, ctx)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusCreated, bookingResponse{ID: bookingID, PassengerName: req.PassengerName, PassengerEmail: req.PassengerEmail, FlightID: req.FlightID, SeatCount: req.SeatCount, TotalPrice: totalPrice, Status: "CONFIRMED"})
}

func cancelBooking(w http.ResponseWriter, r *http.Request, db *pgxpool.Pool, client flightpb.FlightServiceClient) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/bookings/"), "/cancel")
	var bookingID, statusValue string
	if err := db.QueryRow(r.Context(), `SELECT id, status FROM bookings WHERE id=$1`, id).Scan(&bookingID, &statusValue); err != nil {
		http.Error(w, "booking not found", http.StatusNotFound)
		return
	}
	if statusValue == "CANCELLED" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	_, err := retryCall(func(ctx context.Context) (*flightpb.ReleaseReservationResponse, error) {
		return client.ReleaseReservation(ctx, &flightpb.ReleaseReservationRequest{BookingId: bookingID})
	}, authCtx(r.Context()))
	if err != nil {
		http.Error(w, err.Error(), statusToHTTP(err))
		return
	}
	if _, err = db.Exec(r.Context(), `UPDATE bookings SET status='CANCELLED', updated_at=NOW() WHERE id=$1`, bookingID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func retryCall[T any](fn func(context.Context) (T, error), baseCtx context.Context) (T, error) {
	var zero T
	backoffs := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	var last error
	for i, backoff := range backoffs {
		ctx, cancel := context.WithTimeout(baseCtx, 2*time.Second)
		res, err := fn(ctx)
		cancel()
		if err == nil {
			return res, nil
		}
		st, ok := status.FromError(err)
		if !ok || (st.Code() != codes.Unavailable && st.Code() != codes.DeadlineExceeded) || i == len(backoffs)-1 {
			return zero, err
		}
		last = err
		time.Sleep(backoff)
	}
	return zero, last
}

func authCtx(ctx context.Context) context.Context {
	return metadata.NewOutgoingContext(ctx, metadata.Pairs("x-api-key", env("FLIGHT_API_KEY", "hw3-secret"), "x-request-ts", timestamppb.Now().AsTime().Format(time.RFC3339)))
}

func statusToHTTP(err error) int {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError
	}
	switch st.Code() {
	case codes.NotFound:
		return http.StatusNotFound
	case codes.ResourceExhausted:
		return http.StatusConflict
	case codes.Unauthenticated:
		return http.StatusUnauthorized
	default:
		return http.StatusBadGateway
	}
}

func respondJSON(w http.ResponseWriter, code int, v any) {
	buf := bytes.Buffer{}
	_ = json.NewEncoder(&buf).Encode(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(buf.Bytes())
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
