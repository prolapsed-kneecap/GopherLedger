// Package router connects all URLs to handlers.
package router

import (
	"net/http"

	"gopherledger/internal/handler"
	"gopherledger/internal/middleware"
)

// New makes the router with all paths.
//
// Public URLs:
//
//	POST /api/user/register
//	POST /api/user/login
//
// Private URLs:
//
//	POST /api/user/orders
//	GET  /api/user/orders
//	GET  /api/user/balance
//	POST /api/user/balance/withdraw
//	GET  /api/user/withdrawals
//	POST /api/stats/export
func New(h *handler.Handler) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/user/register", h.Register)
	mux.HandleFunc("POST /api/user/login", h.Login)

	mux.Handle("POST /api/user/orders", middleware.Auth(http.HandlerFunc(h.CreateOrder)))
	mux.Handle("GET /api/user/orders", middleware.Auth(http.HandlerFunc(h.GetOrders)))
	mux.Handle("GET /api/user/balance", middleware.Auth(http.HandlerFunc(h.GetBalance)))
	mux.Handle("POST /api/user/balance/withdraw", middleware.Auth(http.HandlerFunc(h.Withdraw)))
	mux.Handle("GET /api/user/withdrawals", middleware.Auth(http.HandlerFunc(h.GetWithdrawals)))
	mux.Handle("POST /api/stats/export", middleware.Auth(http.HandlerFunc(h.ExportStats)))

	handler := middleware.Recover(middleware.Logging(mux))

	return handler
}
