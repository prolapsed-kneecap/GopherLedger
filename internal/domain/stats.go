package domain

import "time"

type Stats struct {
	TotalUsers     int            `json:"total_users"`
	TotalOrders    int            `json:"total_orders"`
	OrdersByStatus map[string]int `json:"orders_by_status"`
	TotalAccrued   float64        `json:"total_accrued"`
	TotalWithdrawn float64        `json:"total_withdrawn"`
	GeneratedAt    time.Time      `json:"generated_at"`
}
