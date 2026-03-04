package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type LogEntry struct {
	RequestID  string  `json:"request_id"`
	Method     string  `json:"method"`
	Endpoint   string  `json:"endpoint"`
	StatusCode int     `json:"status_code"`
	DurationMs int64   `json:"duration_ms"`
	UserID     *string `json:"user_id"`
	Timestamp  string  `json:"timestamp"`
}

func JSONLoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := middleware.GetReqID(r.Context())

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		ww.Header().Set("X-Request-Id", reqID)
		next.ServeHTTP(ww, r)

		duration := time.Since(start).Milliseconds()

		var userID *string
		uid, ok := r.Context().Value("user_id").(string)
		if ok {
			userID = &uid
		}

		entry := LogEntry{
			RequestID:  reqID,
			Method:     r.Method,
			Endpoint:   r.URL.Path,
			StatusCode: ww.Status(),
			DurationMs: duration,
			UserID:     userID,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		}

		logBytes, _ := json.Marshal(entry)
		println(string(logBytes))
	})
}

func isAuthRoute(path string) bool {
	return strings.HasPrefix(path, "/api/v1/auth/")
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isAuthRoute(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			RespondErr(w, http.StatusUnauthorized, "TOKEN_INVALID", "missing or invalid authorization header")
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := ValidateToken(tokenStr, "access")
		if err != nil {
			errCode := "TOKEN_INVALID"
			if strings.Contains(err.Error(), "expired") {
				errCode = "TOKEN_EXPIRED"
			}
			RespondErr(w, http.StatusUnauthorized, errCode, err.Error())
			return
		}

		ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
		ctx = context.WithValue(ctx, "role", claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func RoleMiddleware(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isAuthRoute(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			userRole, ok := r.Context().Value("role").(string)
			if !ok {
				RespondErr(w, http.StatusForbidden, "ACCESS_DENIED", "no role found")
				return
			}

			hasAccess := false
			for _, role := range roles {
				if userRole == role {
					hasAccess = true
					break
				}
			}

			if !hasAccess {
				RespondErr(w, http.StatusForbidden, "ACCESS_DENIED", "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
