package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMiddleware_NoSecret_Passthrough(t *testing.T) {
	h := Middleware("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserID(r.Context()) == "" {
			t.Fatal("expected dev user id")
		}
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 got %d", rec.Code)
	}
}

func TestMiddleware_ValidToken(t *testing.T) {
	secret := "test-secret-32-chars-long-123456"
	claims := Claims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user-123", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	token, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	h := Middleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if UserID(r.Context()) != "user-123" {
			t.Fatalf("wrong user %s", UserID(r.Context()))
		}
		w.WriteHeader(200)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_InvalidToken_401(t *testing.T) {
	h := Middleware("secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("expected 401 got %d", rec.Code)
	}
}
