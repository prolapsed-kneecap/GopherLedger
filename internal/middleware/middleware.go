// Package middleware has HTTP helpers for auth, logs, and panics.
package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"gopherledger/internal/auth"
	"gopherledger/internal/handler"
)

var LogLevel = "info"

// Auth checks the user token and saves the user ID.
// It blocks bad tokens.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		token = strings.TrimPrefix(token, "Bearer ")
		token = strings.TrimSpace(token)
		
		if token == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "UNAUTHORIZED", "message": "не авторизован"})
			return
		}
		userID, err := auth.ValidateToken(token)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "UNAUTHORIZED", "message": "не авторизован"})
			return
		}

		ctx := context.WithValue(r.Context(), handler.CtxKeyUserID, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder saves the HTTP status code.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.wroteHeader = true
		r.ResponseWriter.WriteHeader(code)
	}
}

// Logging prints info about every request.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)
		dur := time.Since(start)
		
		if LogLevel == "debug" {
			log.Printf("method=%s path=%s status=%d duration=%v remote=%s user_agent=%q",
				r.Method, r.URL.Path, recorder.status, dur, r.RemoteAddr, r.UserAgent())
		} else {
			log.Printf("method=%s path=%s status=%d time=%v", r.Method, r.URL.Path, recorder.status, dur)
		}
	})
}

// Recover stops the server from crashing on panics.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			err := recover()
			if err != nil {
				log.Printf("PANIC: %v", err)
				debug.PrintStack()
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
