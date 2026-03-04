package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/username/hw2/api"
)

type ProductSnapshot struct {
	ID     openapi_types.UUID
	Price  float32
	Stock  int
	Status string
}

func (s *Server) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req api.OrderCreate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid json")
		return
	}

	userIDStr, ok := r.Context().Value("user_id").(string)
	if !ok {
		RespondErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing auth")
		return
	}
	parsedUUID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid user_id")
		return
	}
	userID := openapi_types.UUID(parsedUUID)

	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		RespondErr(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}
	defer tx.Rollback(ctx)

	var lastOpTime time.Time
	err = tx.QueryRow(ctx, "SELECT created_at FROM user_operations WHERE user_id = $1 AND operation_type = 'CREATE_ORDER' ORDER BY created_at DESC LIMIT 1", userID).Scan(&lastOpTime)
	if err == nil && time.Since(lastOpTime) < 1*time.Minute {
		RespondErr(w, http.StatusTooManyRequests, "ORDER_LIMIT_EXCEEDED", "too many orders")
		return
	}

	var activeCount int
	err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM orders WHERE user_id = $1 AND status IN ('CREATED', 'PAYMENT_PENDING')", userID).Scan(&activeCount)
	if err == nil && activeCount > 0 {
		RespondErr(w, http.StatusConflict, "ORDER_HAS_ACTIVE", "user already has an active order")
		return
	}

	var totalAmount float32
	details := make(map[string]interface{})
	hasErrors := false

	for _, item := range req.Items {
		var p ProductSnapshot
		err = tx.QueryRow(ctx, "SELECT id, price, stock, status FROM products WHERE id = $1 FOR UPDATE", item.ProductId).Scan(&p.ID, &p.Price, &p.Stock, &p.Status)
		if errors.Is(err, pgx.ErrNoRows) {
			RespondErr(w, http.StatusNotFound, "PRODUCT_NOT_FOUND", "product not found")
			return
		}

		if p.Status != "ACTIVE" {
			RespondErr(w, http.StatusConflict, "PRODUCT_INACTIVE", "product inactive: "+item.ProductId.String())
			return
		}

		if p.Stock < item.Quantity {
			details[item.ProductId.String()] = map[string]int{"requested": item.Quantity, "available": p.Stock}
			hasErrors = true
		} else {
			totalAmount += p.Price * float32(item.Quantity)
			_, err = tx.Exec(ctx, "UPDATE products SET stock = stock - $1 WHERE id = $2", item.Quantity, item.ProductId)
			if err != nil {
				RespondErr(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
				return
			}
		}
	}

	if hasErrors {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(api.ErrorResponse{
			ErrorCode: "INSUFFICIENT_STOCK",
			Message:   "insufficient stock",
			Details:   &details,
		})
		return
	}

	var discountAmount float32 = 0
	var promoID *openapi_types.UUID
	if req.PromoCode != nil && *req.PromoCode != "" {
		var pID openapi_types.UUID
		var active bool
		var curUses, maxUses int
		var vFrom, vUntil time.Time
		var dType string
		var dVal, minAmt float32

		err = tx.QueryRow(ctx, "SELECT id, active, current_uses, max_uses, valid_from, valid_until, discount_type, discount_value, min_order_amount FROM promo_codes WHERE code = $1 FOR UPDATE", *req.PromoCode).
			Scan(&pID, &active, &curUses, &maxUses, &vFrom, &vUntil, &dType, &dVal, &minAmt)
		if err != nil {
			RespondErr(w, http.StatusUnprocessableEntity, "PROMO_CODE_INVALID", "invalid promo code")
			return
		}

		if !active || curUses >= maxUses || time.Now().Before(vFrom) || time.Now().After(vUntil) {
			RespondErr(w, http.StatusUnprocessableEntity, "PROMO_CODE_INVALID", "expired or inactive promo code")
			return
		}

		if totalAmount < minAmt {
			RespondErr(w, http.StatusUnprocessableEntity, "PROMO_CODE_MIN_AMOUNT", "order amount too low for promo code")
			return
		}

		if dType == "PERCENTAGE" {
			discount := totalAmount * dVal / 100
			if discount > totalAmount*0.7 {
				discountAmount = totalAmount * 0.7
			} else {
				discountAmount = discount
			}
		} else {
			if dVal > totalAmount {
				discountAmount = totalAmount
			} else {
				discountAmount = dVal
			}
		}

		totalAmount -= discountAmount
		promoID = &pID

		_, err = tx.Exec(ctx, "UPDATE promo_codes SET current_uses = current_uses + 1 WHERE id = $1", pID)
		if err != nil {
			RespondErr(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
			return
		}
	}

	var orderID openapi_types.UUID
	var createdAt, updatedAt time.Time
	err = tx.QueryRow(ctx,
		"INSERT INTO orders (user_id, status, promo_code_id, total_amount, discount_amount) VALUES ($1, $2, $3, $4, $5) RETURNING id, created_at, updated_at",
		userID, "CREATED", promoID, totalAmount, discountAmount).Scan(&orderID, &createdAt, &updatedAt)
	if err != nil {
		RespondErr(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	for _, item := range req.Items {
		var price float32
		tx.QueryRow(ctx, "SELECT price FROM products WHERE id = $1", item.ProductId).Scan(&price)
		_, err = tx.Exec(ctx, "INSERT INTO order_items (order_id, product_id, quantity, price_at_order) VALUES ($1, $2, $3, $4)",
			orderID, item.ProductId, item.Quantity, price)
		if err != nil {
			RespondErr(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
			return
		}
	}

	_, err = tx.Exec(ctx, "INSERT INTO user_operations (user_id, operation_type) VALUES ($1, 'CREATE_ORDER')", userID)
	if err != nil {
		RespondErr(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	if err := tx.Commit(ctx); err != nil {
		RespondErr(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(api.OrderResponse{
		Id:             orderID,
		UserId:         userID,
		Status:         "CREATED",
		PromoCodeId:    promoID,
		TotalAmount:    totalAmount,
		DiscountAmount: discountAmount,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	})
}

func (s *Server) GetOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var o api.OrderResponse
	o.Id = id
	var status string
	err := s.DB.QueryRow(context.Background(), "SELECT user_id, status, promo_code_id, total_amount, discount_amount, created_at, updated_at FROM orders WHERE id = $1", id).
		Scan(&o.UserId, &status, &o.PromoCodeId, &o.TotalAmount, &o.DiscountAmount, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		RespondErr(w, http.StatusNotFound, "ORDER_NOT_FOUND", "order not found")
		return
	}
	o.Status = api.OrderResponseStatus(status)

	uidStr, _ := r.Context().Value("user_id").(string)
	role, _ := r.Context().Value("role").(string)
	if role != "ADMIN" && uidStr != o.UserId.String() {
		RespondErr(w, http.StatusForbidden, "ORDER_OWNERSHIP_VIOLATION", "order not yours")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(o)
}

func (s *Server) UpdateOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var req api.OrderUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid json")
		return
	}

	uidStr, _ := r.Context().Value("user_id").(string)
	parsedUUID, _ := uuid.Parse(uidStr)
	userID := openapi_types.UUID(parsedUUID)
	role, _ := r.Context().Value("role").(string)

	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		RespondErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	defer tx.Rollback(ctx)

	var ownerID openapi_types.UUID
	var status string
	var promoID *openapi_types.UUID
	err = tx.QueryRow(ctx, "SELECT user_id, status, promo_code_id FROM orders WHERE id = $1 FOR UPDATE", id).Scan(&ownerID, &status, &promoID)
	if err != nil {
		RespondErr(w, http.StatusNotFound, "ORDER_NOT_FOUND", "order not found")
		return
	}

	if role != "ADMIN" && ownerID != userID {
		RespondErr(w, http.StatusForbidden, "ORDER_OWNERSHIP_VIOLATION", "not yours")
		return
	}

	if status != "CREATED" {
		RespondErr(w, http.StatusConflict, "INVALID_STATE_TRANSITION", "can only update CREATED order")
		return
	}

	var lastOpTime time.Time
	err = tx.QueryRow(ctx, "SELECT created_at FROM user_operations WHERE user_id = $1 AND operation_type = 'UPDATE_ORDER' ORDER BY created_at DESC LIMIT 1", userID).Scan(&lastOpTime)
	if err == nil && time.Since(lastOpTime) < 1*time.Minute {
		RespondErr(w, http.StatusTooManyRequests, "ORDER_LIMIT_EXCEEDED", "too many updates")
		return
	}

	rows, err := tx.Query(ctx, "SELECT product_id, quantity FROM order_items WHERE order_id = $1", id)
	if err != nil {
		RespondErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	var oldItems []api.OrderItemCreate
	for rows.Next() {
		var i api.OrderItemCreate
		rows.Scan(&i.ProductId, &i.Quantity)
		oldItems = append(oldItems, i)
	}
	rows.Close()

	for _, oi := range oldItems {
		_, err = tx.Exec(ctx, "UPDATE products SET stock = stock + $1 WHERE id = $2", oi.Quantity, oi.ProductId)
		if err != nil {
			RespondErr(w, 500, "DB_ERROR", err.Error())
			return
		}
	}
	_, err = tx.Exec(ctx, "DELETE FROM order_items WHERE order_id = $1", id)

	var totalAmount float32
	details := make(map[string]interface{})
	hasErrors := false

	for _, item := range req.Items {
		var p ProductSnapshot
		err = tx.QueryRow(ctx, "SELECT price, stock, status FROM products WHERE id = $1 FOR UPDATE", item.ProductId).Scan(&p.Price, &p.Stock, &p.Status)
		if errors.Is(err, pgx.ErrNoRows) || p.Status != "ACTIVE" {
			hasErrors = true
			RespondErr(w, http.StatusConflict, "PRODUCT_INACTIVE", "product not found or inactive")
			return
		}
		if p.Stock < item.Quantity {
			details[item.ProductId.String()] = map[string]int{"requested": item.Quantity, "available": p.Stock}
			hasErrors = true
		} else {
			totalAmount += p.Price * float32(item.Quantity)
			tx.Exec(ctx, "UPDATE products SET stock = stock - $1 WHERE id = $2", item.Quantity, item.ProductId)
		}
	}

	if hasErrors {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(api.ErrorResponse{
			ErrorCode: "INSUFFICIENT_STOCK",
			Message:   "insufficient stock",
			Details:   &details,
		})
		return
	}

	for _, item := range req.Items {
		var price float32
		tx.QueryRow(ctx, "SELECT price FROM products WHERE id = $1", item.ProductId).Scan(&price)
		tx.Exec(ctx, "INSERT INTO order_items (order_id, product_id, quantity, price_at_order) VALUES ($1, $2, $3, $4)",
			id, item.ProductId, item.Quantity, price)
	}

	var discountAmount float32 = 0
	if promoID != nil {
		var active bool
		var curUses, maxUses int
		var vFrom, vUntil time.Time
		var dType string
		var dVal, minAmt float32
		err = tx.QueryRow(ctx, "SELECT active, current_uses, max_uses, valid_from, valid_until, discount_type, discount_value, min_order_amount FROM promo_codes WHERE id = $1 FOR UPDATE", promoID).
			Scan(&active, &curUses, &maxUses, &vFrom, &vUntil, &dType, &dVal, &minAmt)

		valid := err == nil && active && time.Now().After(vFrom) && time.Now().Before(vUntil) && totalAmount >= minAmt
		if valid {
			if dType == "PERCENTAGE" {
				discountAmount = totalAmount * dVal / 100
				if discountAmount > totalAmount*0.7 {
					discountAmount = totalAmount * 0.7
				}
			} else {
				discountAmount = dVal
				if discountAmount > totalAmount {
					discountAmount = totalAmount
				}
			}
			totalAmount -= discountAmount
		} else {
			oldPromoID := promoID
			promoID = nil
			tx.Exec(ctx, "UPDATE promo_codes SET current_uses = current_uses - 1 WHERE id = $1", oldPromoID)
		}
	}

	_, err = tx.Exec(ctx, "UPDATE orders SET total_amount = $1, discount_amount = $2, promo_code_id = $3, updated_at = NOW() WHERE id = $4",
		totalAmount, discountAmount, promoID, id)
	tx.Exec(ctx, "INSERT INTO user_operations (user_id, operation_type) VALUES ($1, 'UPDATE_ORDER')", userID)

	if err := tx.Commit(ctx); err != nil {
		RespondErr(w, 500, "DB_ERROR", err.Error())
		return
	}

	s.GetOrder(w, r, id)
}

func (s *Server) CancelOrder(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	uidStr, _ := r.Context().Value("user_id").(string)
	parsedUUID, _ := uuid.Parse(uidStr)
	userID := openapi_types.UUID(parsedUUID)
	role, _ := r.Context().Value("role").(string)

	ctx := context.Background()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		RespondErr(w, 500, "DB_ERROR", err.Error())
		return
	}
	defer tx.Rollback(ctx)

	var ownerID openapi_types.UUID
	var status string
	var promoID *openapi_types.UUID
	err = tx.QueryRow(ctx, "SELECT user_id, status, promo_code_id FROM orders WHERE id = $1 FOR UPDATE", id).Scan(&ownerID, &status, &promoID)
	if err != nil {
		RespondErr(w, http.StatusNotFound, "ORDER_NOT_FOUND", "not found")
		return
	}
	if role != "ADMIN" && ownerID != userID {
		RespondErr(w, http.StatusForbidden, "ORDER_OWNERSHIP_VIOLATION", "not yours")
		return
	}
	if status != "CREATED" && status != "PAYMENT_PENDING" {
		RespondErr(w, http.StatusConflict, "INVALID_STATE_TRANSITION", "must be CREATED or PAYMENT_PENDING")
		return
	}

	rows, _ := tx.Query(ctx, "SELECT product_id, quantity FROM order_items WHERE order_id = $1", id)
	var oldItems []api.OrderItemCreate
	for rows.Next() {
		var i api.OrderItemCreate
		rows.Scan(&i.ProductId, &i.Quantity)
		oldItems = append(oldItems, i)
	}
	rows.Close()

	for _, oi := range oldItems {
		tx.Exec(ctx, "UPDATE products SET stock = stock + $1 WHERE id = $2", oi.Quantity, oi.ProductId)
	}

	if promoID != nil {
		tx.Exec(ctx, "UPDATE promo_codes SET current_uses = current_uses - 1 WHERE id = $1", promoID)
	}

	tx.Exec(ctx, "UPDATE orders SET status = 'CANCELED', updated_at = NOW() WHERE id = $1", id)

	if err := tx.Commit(ctx); err != nil {
		RespondErr(w, 500, "DB_ERROR", err.Error())
		return
	}

	s.GetOrder(w, r, id)
}
