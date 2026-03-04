package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/username/hw2/api"
)

type Server struct {
	DB *pgxpool.Pool
}

func NewServer(db *pgxpool.Pool) *Server {
	return &Server{DB: db}
}

func RespondErr(w http.ResponseWriter, code int, errCode, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(api.ErrorResponse{
		ErrorCode: errCode,
		Message:   msg,
	})
}

func (s *Server) ListProducts(w http.ResponseWriter, r *http.Request, params api.ListProductsParams) {
	w.Header().Set("Content-Type", "application/json")

	rows, err := s.DB.Query(context.Background(), "SELECT id, name, description, price, stock, category, status, seller_id, created_at, updated_at FROM products")
	if err != nil {
		RespondErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	defer rows.Close()

	var products []api.ProductResponse
	for rows.Next() {
		var p api.ProductResponse
		var desc *string
		if err := rows.Scan(&p.Id, &p.Name, &desc, &p.Price, &p.Stock, &p.Category, &p.Status, &p.SellerId, &p.CreatedAt, &p.UpdatedAt); err != nil {
			RespondErr(w, 500, "DB_ERROR", err.Error())
			return
		}
		p.Description = desc
		products = append(products, p)
	}

	res := api.ProductListResponse{
		PageNumber:    0,
		PageSize:      len(products),
		Products:      products,
		TotalElements: len(products),
	}
	json.NewEncoder(w).Encode(res)
}

func (s *Server) CreateProduct(w http.ResponseWriter, r *http.Request) {
	var req api.ProductCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}

	var p api.ProductResponse
	uidStr, ok := r.Context().Value("user_id").(string)
	if !ok {
		RespondErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing user_id")
		return
	}
	sellerIdUUID, err := uuid.Parse(uidStr)
	if err != nil {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid user_id")
		return
	}
	sellerId := openapi_types.UUID(sellerIdUUID)

	var pID uuid.UUID
	var created, updated time.Time
	err = s.DB.QueryRow(context.Background(),
		`INSERT INTO products (name, description, price, stock, category, status, seller_id) 
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, created_at, updated_at`,
		req.Name, req.Description, req.Price, req.Stock, req.Category, req.Status, sellerId).Scan(&pID, &created, &updated)

	if err != nil {
		RespondErr(w, 500, "DB_ERROR", err.Error())
		return
	}

	p.Id = openapi_types.UUID(pID)
	p.CreatedAt = created
	p.UpdatedAt = updated
	p.Name = req.Name
	p.Description = req.Description
	p.Price = req.Price
	p.Stock = req.Stock
	p.Category = req.Category
	p.Status = api.ProductResponseStatus(req.Status)
	p.SellerId = sellerId

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

func (s *Server) DeleteProduct(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	_, err := s.DB.Exec(context.Background(), "UPDATE products SET status = 'ARCHIVED', updated_at = $1 WHERE id = $2", time.Now(), id)
	if err != nil {
		RespondErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) GetProduct(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var p api.ProductResponse
	var desc *string
	err := s.DB.QueryRow(context.Background(),
		"SELECT id, name, description, price, stock, category, status, seller_id, created_at, updated_at FROM products WHERE id = $1", id).
		Scan(&p.Id, &p.Name, &desc, &p.Price, &p.Stock, &p.Category, &p.Status, &p.SellerId, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		RespondErr(w, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Product not found")
		return
	}
	p.Description = desc

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (s *Server) UpdateProduct(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	RespondErr(w, http.StatusNotImplemented, "NOT_IMPLEMENTED", "TODO")
}

func (s *Server) CreatePromoCode(w http.ResponseWriter, r *http.Request) {
	var req api.PromoCodeCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON")
		return
	}

	var pc api.PromoCodeResponse
	var pcID uuid.UUID
	err := s.DB.QueryRow(context.Background(),
		`INSERT INTO promo_codes (code, discount_type, discount_value, min_order_amount, max_uses, valid_from, valid_until) 
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		req.Code, req.DiscountType, req.DiscountValue, req.MinOrderAmount, req.MaxUses, req.ValidFrom, req.ValidUntil).Scan(&pcID)

	if err != nil {
		RespondErr(w, http.StatusConflict, "DB_ERROR", err.Error())
		return
	}

	pc.Id = openapi_types.UUID(pcID)
	pc.Code = req.Code
	pc.DiscountType = string(req.DiscountType)
	pc.DiscountValue = req.DiscountValue
	pc.MinOrderAmount = req.MinOrderAmount
	pc.MaxUses = req.MaxUses
	pc.CurrentUses = 0
	pc.ValidFrom = req.ValidFrom
	pc.ValidUntil = req.ValidUntil
	pc.Active = true

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(pc)
}
