package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/username/hw2/api"
	serverAPI "github.com/username/hw2/internal/api"
)

func main() {
	dbUrl := os.Getenv("DB_URL")
	if dbUrl == "" {
		dbUrl = "postgres://user:password@localhost:5432/marketplace?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), dbUrl)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer pool.Close()

	srv := serverAPI.NewServer(pool)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(serverAPI.JSONLoggerMiddleware)
	r.Use(middleware.Recoverer)

	errWrapper := func(w http.ResponseWriter, r *http.Request, err error) {
		serverAPI.RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	}

	wrapper := api.ServerInterfaceWrapper{
		Handler:          srv,
		ErrorHandlerFunc: errWrapper,
	}

	r.Group(func(r chi.Router) {
		r.Post("/api/v1/auth/login", wrapper.AuthLogin)
		r.Post("/api/v1/auth/refresh", wrapper.AuthRefresh)
		r.Post("/api/v1/auth/register", wrapper.AuthRegister)
	})

	r.Group(func(r chi.Router) {
		r.Use(serverAPI.AuthMiddleware)

		r.Get("/api/v1/products", wrapper.ListProducts)
		r.Get("/api/v1/products/{id}", wrapper.GetProduct)
		r.Get("/api/v1/orders/{id}", wrapper.GetOrder)

		r.Group(func(r chi.Router) {
			r.Use(serverAPI.RoleMiddleware("USER", "ADMIN"))
			r.Post("/api/v1/orders", wrapper.CreateOrder)
			r.Put("/api/v1/orders/{id}", wrapper.UpdateOrder)
			r.Post("/api/v1/orders/{id}/cancel", wrapper.CancelOrder)
		})

		r.Group(func(r chi.Router) {
			r.Use(serverAPI.RoleMiddleware("SELLER", "ADMIN"))
			r.Post("/api/v1/products", wrapper.CreateProduct)
			r.Put("/api/v1/products/{id}", wrapper.UpdateProduct)
			r.Delete("/api/v1/products/{id}", wrapper.DeleteProduct)
		})

		r.Group(func(r chi.Router) {
			r.Use(serverAPI.RoleMiddleware("SELLER", "ADMIN"))
			r.Post("/api/v1/promo-codes", wrapper.CreatePromoCode)
		})
	})

	log.Println("Starting server on :8080...")
	log.Fatal(http.ListenAndServe(":8080", r))
}
