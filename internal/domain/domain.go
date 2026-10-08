// Package domain has the main types and errors.
package domain

import (
	"errors"
	"time"
)

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

var (
	ErrUserExists        = errors.New("пользователь уже существует")
	ErrUserNotFound      = errors.New("пользователь не найден")
	ErrInvalidPassword   = errors.New("неверный пароль")
	ErrOrderExists       = errors.New("заказ уже загружен другим пользователем")
	ErrOrderOwnedByUser  = errors.New("заказ уже загружен этим пользователем")
	ErrInsufficientFunds = errors.New("недостаточно баллов")
	ErrInvalidOrder      = errors.New("неверный номер заказа")
)

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// User is someone using the app.
type User struct {
	ID           int64
	Login        string
	PasswordHash string
}

// Order is a user's purchase.
type Order struct {
	ID         int64
	UserID     int64
	Number     string
	Status     string
	Accrual    float64
	UploadedAt time.Time
}

// Balance is the user's points.
type Balance struct {
	Current   float64
	Withdrawn float64
}

// Withdrawal is a record of spent points.
type Withdrawal struct {
	ID          int64
	UserID      int64
	OrderNumber string
	Sum         float64
	ProcessedAt time.Time
}

// ---------------------------------------------------------------------------
// Order statuses
// ---------------------------------------------------------------------------

const (
	OrderStatusNew        = "NEW"
	OrderStatusProcessing = "PROCESSING"
	OrderStatusInvalid    = "INVALID"
	OrderStatusProcessed  = "PROCESSED"
)
