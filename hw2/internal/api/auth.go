package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/username/hw2/api"
)

func (s *Server) AuthRegister(w http.ResponseWriter, r *http.Request) {
	var req api.AuthRegister
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" || req.Role == "" {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "missing required fields")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		RespondErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to hash password")
		return
	}

	_, err = s.DB.Exec(context.Background(),
		"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, $3)",
		req.Username, string(hash), string(req.Role),
	)
	if err != nil {
		RespondErr(w, http.StatusConflict, "USER_EXISTS", "user already exists")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) AuthLogin(w http.ResponseWriter, r *http.Request) {
	var req api.AuthLogin
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	var userID, role, hash string
	err := s.DB.QueryRow(context.Background(),
		"SELECT id, role, password_hash FROM users WHERE username = $1", req.Username).
		Scan(&userID, &role, &hash)

	if errors.Is(err, pgx.ErrNoRows) {
		RespondErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	} else if err != nil {
		RespondErr(w, http.StatusInternalServerError, "DB_ERROR", err.Error())
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		RespondErr(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid credentials")
		return
	}

	accToken, refToken, err := GenerateTokens(userID, role)
	if err != nil {
		RespondErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate tokens")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.AuthTokens{
		AccessToken:  accToken,
		RefreshToken: refToken,
	})
}

func (s *Server) AuthRefresh(w http.ResponseWriter, r *http.Request) {
	var req api.AuthRefresh
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid request body")
		return
	}

	claims, err := ValidateToken(req.RefreshToken, "refresh")
	if err != nil {
		RespondErr(w, http.StatusUnauthorized, "REFRESH_TOKEN_INVALID", "invalid or expired refresh token")
		return
	}

	accToken, refToken, err := GenerateTokens(claims.UserID, claims.Role)
	if err != nil {
		RespondErr(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to generate tokens")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(api.AuthTokens{
		AccessToken:  accToken,
		RefreshToken: refToken,
	})
}
