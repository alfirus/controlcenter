package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type ctxKey string

const UserIDKey ctxKey = "user_id"
const RoleKey ctxKey = "role"

type Claims struct {
	jwt.RegisteredClaims
	Role  string `json:"role"`
	Email string `json:"email"`
}

// Middleware validates Supabase JWT (HS256 with SUPABASE_JWT_SECRET).
// Expects Authorization: Bearer <token>. On failure, 401.
// Skips if secret is empty (dev mode) — injects dev user.
func Middleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				ctx := context.WithValue(r.Context(), UserIDKey, "00000000-0000-0000-0000-000000000001")
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			h := r.Header.Get("Authorization")
			if h == "" {
				http.Error(w, `{"error":{"code":"unauthorized","message":"missing Authorization"}}`, http.StatusUnauthorized)
				return
			}
			tokenStr := strings.TrimPrefix(h, "Bearer ")
			if tokenStr == h {
				http.Error(w, `{"error":{"code":"unauthorized","message":"invalid Authorization header"}}`, http.StatusUnauthorized)
				return
			}
			token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, errors.New("unexpected signing method")
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, `{"error":{"code":"unauthorized","message":"invalid token"}}`, http.StatusUnauthorized)
				return
			}
			claims, ok := token.Claims.(*Claims)
			if !ok || claims.Subject == "" {
				http.Error(w, `{"error":{"code":"unauthorized","message":"invalid claims"}}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), UserIDKey, claims.Subject)
			ctx = context.WithValue(ctx, RoleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserID(ctx context.Context) string {
	v, _ := ctx.Value(UserIDKey).(string)
	return v
}

// OptionalAuth — like Middleware but does not 401 if missing (for /healthz passthrough); used only if needed.
func OptionalAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h == "" || secret == "" {
				next.ServeHTTP(w, r)
				return
			}
			Middleware(secret)(next).ServeHTTP(w, r)
		})
	}
}
