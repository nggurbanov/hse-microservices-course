package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	flightpb "github.com/nggurbanov/hse-microservices-course/hw3/gen/flightpb"
)

type server struct {
	flightpb.UnimplementedFlightServiceServer
	db    *pgxpool.Pool
	redis *redis.Client
}

type flightRow struct {
	ID             string
	FlightNumber   string
	Airline        string
	Origin         string
	Destination    string
	DepartureTime  time.Time
	ArrivalTime    time.Time
	TotalSeats     int32
	AvailableSeats int32
	Price          float64
	Status         string
}

func main() {
	db, err := pgxpool.New(context.Background(), os.Getenv("DB_URL"))
	if err != nil {
		log.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_ADDR")})
	lis, err := net.Listen("tcp", env("GRPC_ADDR", ":50051"))
	if err != nil {
		log.Fatal(err)
	}
	s := grpc.NewServer(grpc.UnaryInterceptor(authInterceptor(env("FLIGHT_API_KEY", "hw3-secret"))))
	flightpb.RegisterFlightServiceServer(s, &server{db: db, redis: rdb})
	log.Fatal(s.Serve(lis))
}

func authInterceptor(secret string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok || len(md.Get("x-api-key")) == 0 || md.Get("x-api-key")[0] != secret {
			return nil, status.Error(codes.Unauthenticated, "invalid api key")
		}
		return handler(ctx, req)
	}
}

func (s *server) SearchFlights(ctx context.Context, req *flightpb.SearchFlightsRequest) (*flightpb.SearchFlightsResponse, error) {
	date := req.DepartureDate.AsTime().Format("2006-01-02")
	key := "search:" + req.Origin + ":" + req.Destination + ":" + date
	if raw, err := s.redis.Get(ctx, key).Result(); err == nil {
		log.Println("cache hit", key)
		var resp flightpb.SearchFlightsResponse
		_ = json.Unmarshal([]byte(raw), &resp)
		return &resp, nil
	}
	log.Println("cache miss", key)
	rows, err := s.db.Query(ctx, `SELECT id, flight_number, airline, origin, destination, departure_time, arrival_time, total_seats, available_seats, price, status
		FROM flights WHERE origin=$1 AND destination=$2 AND DATE(departure_time)=DATE($3) ORDER BY departure_time`, req.Origin, req.Destination, req.DepartureDate.AsTime())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer rows.Close()
	resp := &flightpb.SearchFlightsResponse{}
	for rows.Next() {
		f, err := scanFlight(rows)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resp.Flights = append(resp.Flights, toProto(f))
	}
	cacheJSON, _ := json.Marshal(resp)
	s.redis.Set(ctx, key, cacheJSON, 5*time.Minute)
	return resp, nil
}

func (s *server) GetFlight(ctx context.Context, req *flightpb.GetFlightRequest) (*flightpb.Flight, error) {
	key := "flight:" + req.FlightId
	if raw, err := s.redis.Get(ctx, key).Result(); err == nil {
		log.Println("cache hit", key)
		var f flightpb.Flight
		_ = json.Unmarshal([]byte(raw), &f)
		return &f, nil
	}
	log.Println("cache miss", key)
	row := s.db.QueryRow(ctx, `SELECT id, flight_number, airline, origin, destination, departure_time, arrival_time, total_seats, available_seats, price, status FROM flights WHERE id=$1`, req.FlightId)
	f, err := scanFlight(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Error(codes.NotFound, "flight not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	resp := toProto(f)
	cacheJSON, _ := json.Marshal(resp)
	s.redis.Set(ctx, key, cacheJSON, 5*time.Minute)
	return resp, nil
}

func (s *server) ReserveSeats(ctx context.Context, req *flightpb.ReserveSeatsRequest) (*flightpb.ReserveSeatsResponse, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer tx.Rollback(ctx)

	var existingID string
	var existingStatus string
	var existingCount int32
	err = tx.QueryRow(ctx, `SELECT id, status, seat_count FROM seat_reservations WHERE booking_id=$1`, req.BookingId).Scan(&existingID, &existingStatus, &existingCount)
	if err == nil && existingStatus == "ACTIVE" {
		return &flightpb.ReserveSeatsResponse{ReservationId: existingID, Status: flightpb.ReservationStatus_RESERVATION_STATUS_ACTIVE, ReservedSeats: existingCount}, tx.Commit(ctx)
	}
	if err != nil && err != pgx.ErrNoRows {
		return nil, status.Error(codes.Internal, err.Error())
	}

	var available int32
	if err = tx.QueryRow(ctx, `SELECT available_seats FROM flights WHERE id=$1 FOR UPDATE`, req.FlightId).Scan(&available); err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Error(codes.NotFound, "flight not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if available < req.SeatCount {
		return nil, status.Error(codes.ResourceExhausted, "not enough seats")
	}
	if _, err = tx.Exec(ctx, `UPDATE flights SET available_seats = available_seats - $1 WHERE id=$2`, req.SeatCount, req.FlightId); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	id := uuid.NewString()
	if _, err = tx.Exec(ctx, `INSERT INTO seat_reservations (id, booking_id, flight_id, seat_count, status) VALUES ($1,$2,$3,$4,'ACTIVE')
		ON CONFLICT (booking_id) DO UPDATE SET status='ACTIVE', seat_count=EXCLUDED.seat_count, updated_at=NOW()`, id, req.BookingId, req.FlightId, req.SeatCount); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.invalidate(ctx, req.FlightId)
	return &flightpb.ReserveSeatsResponse{ReservationId: id, Status: flightpb.ReservationStatus_RESERVATION_STATUS_ACTIVE, ReservedSeats: req.SeatCount}, nil
}

func (s *server) ReleaseReservation(ctx context.Context, req *flightpb.ReleaseReservationRequest) (*flightpb.ReleaseReservationResponse, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer tx.Rollback(ctx)

	var id, flightID, resStatus string
	var seatCount int32
	err = tx.QueryRow(ctx, `SELECT id, flight_id, seat_count, status FROM seat_reservations WHERE booking_id=$1 FOR UPDATE`, req.BookingId).Scan(&id, &flightID, &seatCount, &resStatus)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, status.Error(codes.NotFound, "reservation not found")
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	if strings.ToUpper(resStatus) != "ACTIVE" {
		return &flightpb.ReleaseReservationResponse{ReservationId: id, Status: flightpb.ReservationStatus_RESERVATION_STATUS_RELEASED}, tx.Commit(ctx)
	}
	if _, err = tx.Exec(ctx, `SELECT id FROM flights WHERE id=$1 FOR UPDATE`, flightID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if _, err = tx.Exec(ctx, `UPDATE flights SET available_seats = available_seats + $1 WHERE id=$2`, seatCount, flightID); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if _, err = tx.Exec(ctx, `UPDATE seat_reservations SET status='RELEASED', updated_at=NOW() WHERE id=$1`, id); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	s.invalidate(ctx, flightID)
	return &flightpb.ReleaseReservationResponse{ReservationId: id, Status: flightpb.ReservationStatus_RESERVATION_STATUS_RELEASED}, nil
}

func (s *server) invalidate(ctx context.Context, flightID string) {
	s.redis.Del(ctx, "flight:"+flightID)
}

func scanFlight(row interface{ Scan(...any) error }) (*flightRow, error) {
	var f flightRow
	err := row.Scan(&f.ID, &f.FlightNumber, &f.Airline, &f.Origin, &f.Destination, &f.DepartureTime, &f.ArrivalTime, &f.TotalSeats, &f.AvailableSeats, &f.Price, &f.Status)
	return &f, err
}

func toProto(f *flightRow) *flightpb.Flight {
	return &flightpb.Flight{
		Id:             f.ID,
		FlightNumber:   f.FlightNumber,
		Airline:        f.Airline,
		Origin:         f.Origin,
		Destination:    f.Destination,
		DepartureTime:  timestamppb.New(f.DepartureTime),
		ArrivalTime:    timestamppb.New(f.ArrivalTime),
		TotalSeats:     f.TotalSeats,
		AvailableSeats: f.AvailableSeats,
		Price:          f.Price,
		Status:         mapFlightStatus(f.Status),
	}
}

func mapFlightStatus(v string) flightpb.FlightStatus {
	switch strings.ToUpper(v) {
	case "SCHEDULED":
		return flightpb.FlightStatus_FLIGHT_STATUS_SCHEDULED
	case "DEPARTED":
		return flightpb.FlightStatus_FLIGHT_STATUS_DEPARTED
	case "CANCELLED":
		return flightpb.FlightStatus_FLIGHT_STATUS_CANCELLED
	case "COMPLETED":
		return flightpb.FlightStatus_FLIGHT_STATUS_COMPLETED
	default:
		return flightpb.FlightStatus_FLIGHT_STATUS_UNSPECIFIED
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
