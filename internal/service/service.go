// Package service contains the main logic. It connects handlers to the store.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"gopherledger/internal/auth"
	"gopherledger/internal/domain"
	"log"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Service holds the logic and the store.
// processingOrders tracks active orders to avoid doing the same work twice.

type Store interface {
	CreateUser(login, passwordHash string) (*domain.User, error)
	GetUserByLogin(login string) (*domain.User, error)
	CreateOrder(userID int64, number string) (*domain.Order, error)
	GetUserOrders(userID int64) ([]domain.Order, error)
	GetOrdersForProcessing() ([]domain.Order, error)
	UpdateOrderStatus(number, status string, accrual float64) error
	GetBalance(userID int64) (domain.Balance, error)
	Withdraw(userID int64, orderNumber string, sum float64) error
	GetWithdrawals(userID int64) ([]domain.Withdrawal, error)
	GetStats() (domain.Stats, error)
}

type Service struct {
	repo             Store
	mu               sync.Mutex
	processingOrders map[string]bool
}

// New creates a Service.
func New(repo Store) *Service {
	return &Service{
		repo:             repo,
		processingOrders: make(map[string]bool),
	}
}

// ---------------------------------------------------------------------------
// Business logic
// ---------------------------------------------------------------------------

// RegisterUser adds a new user and returns a token.
// We hash the password before saving.
func (s *Service) RegisterUser(login, password string) (string, error) {
	hash := sha256.Sum256([]byte(password))
	passwordHash := hex.EncodeToString(hash[:])

	user, err := s.repo.CreateUser(login, passwordHash)

	if err != nil {
		return "", err
	}

	return auth.GenerateToken(user.ID)
}

// LoginUser checks the password and returns a token.
func (s *Service) LoginUser(login, password string) (string, error) {
	user, err := s.repo.GetUserByLogin(login)
	if err != nil {
		return "", domain.ErrUserNotFound
	}
	hash := sha256.Sum256([]byte(password))
	passwordHash := hex.EncodeToString(hash[:])

	if user.PasswordHash != passwordHash {
		return "", domain.ErrInvalidPassword
	}
	return auth.GenerateToken(user.ID)
}

// CreateOrder checks the order number and saves it.
func (s *Service) CreateOrder(userID int64, number string) (*domain.Order, error) {
	if !validateLuhn(number) {
		return nil, domain.ErrInvalidOrder
	}
	return s.repo.CreateOrder(userID, number)
}

// GetUserOrders gets all orders for a user.
func (s *Service) GetUserOrders(userID int64) ([]domain.Order, error) {
	return s.repo.GetUserOrders(userID)
}

// GetBalance gets the user's current balance.
func (s *Service) GetBalance(userID int64) (domain.Balance, error) {
	return s.repo.GetBalance(userID)
}

// Withdraw checks the order number and takes points from the balance.
func (s *Service) Withdraw(userID int64, orderNumber string, sum float64) error {
	if !validateLuhn(orderNumber) {
		return domain.ErrInvalidOrder
	}
	return s.repo.Withdraw(userID, orderNumber, sum)
}

// GetWithdrawals gets the user's past withdrawals.
func (s *Service) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	return s.repo.GetWithdrawals(userID)
}

func (s *Service) GetStats() (domain.Stats, error) {
	return s.repo.GetStats()
}

// validateLuhn checks if an order number is valid.
// We use this when saving orders and spending points.
func validateLuhn(number string) bool {
	if len(number) == 0 {
		return false
	}
	for _, r := range number {
		if (r < '0') || (r > '9') {
			return false
		}
	}
	sum := 0
	runes := []rune(number)
	for i := len(runes) - 1; i >= 0; i-- {
		if (i % 2) != len(runes)%2 {
			sum += int(runes[i] - '0')
			continue
		}
		cur := int(runes[i]-'0') * 2
		if cur >= 10 {
			cur = cur%10 + (cur/10)%10
		}
		sum += cur
	}
	return (sum % 10) == 0
}

// ---------------------------------------------------------------------------
// Background worker
// ---------------------------------------------------------------------------

// StartAccrualWorker runs in the background to process orders.
// It stops on context cancel.
func (s *Service) StartAccrualWorker(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processAllPendingOrders(ctx)
		}
	}
}

// processAllPendingOrders gets new orders and starts working on them.
func (s *Service) processAllPendingOrders(ctx context.Context) {

	orders, err := s.repo.GetOrdersForProcessing()
	if err != nil {
		log.Printf("воркер: ошибка получения заказов: %v", err)
		return
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	// Skip active orders and start new ones.

	for _, order := range orders {
		orderNumber := order.Number
		s.mu.Lock()
		_, ok := s.processingOrders[orderNumber]
		if ok {
			s.mu.Unlock()
			continue
		}
		s.processingOrders[orderNumber] = true
		s.mu.Unlock()

		g.Go(func() error {
			defer func() {
				s.mu.Lock()
				delete(s.processingOrders, orderNumber)
				s.mu.Unlock()
			}()
			s.processOrder(gctx, orderNumber)
			return nil
		})

	}
	if err := g.Wait(); err != nil {
		log.Printf("воркер: ошибка группы: %v", err)
		return
	}
}

// processOrder updates the order status and adds points.
func (s *Service) processOrder(ctx context.Context, number string) {
	err := s.repo.UpdateOrderStatus(number, domain.OrderStatusProcessing, 0)
	if err != nil {
		log.Printf("воркер: ошибка обновления статуса заказа %s: %v", number, err)
		return
	}

	select {
	case <-time.After(randomDelay()):
	case <-ctx.Done():
		log.Printf("воркер: обработка заказа %s отменена", number)
		return
	}

	if isInvalid() {
		err := s.repo.UpdateOrderStatus(number, domain.OrderStatusInvalid, 0)
		if err != nil {
			log.Printf("воркер: ошибка обновления статуса заказа %s: %v", number, err)
		}
		return
	}

	accural := randomAccrual()
	err = s.repo.UpdateOrderStatus(number, domain.OrderStatusProcessed, accural)

	if err != nil {
		log.Printf("воркер: ошибка обновления статуса заказа %s: %v", number, err)
		return
	}

	log.Printf("воркер: заказ %s обработан, начислено %.2f баллов", number, accural)

}

// ---------------------------------------------------------------------------
// Helper functions - provided
// ---------------------------------------------------------------------------

// randomAccrual gives a random amount of points.
func randomAccrual() float64 {
	return float64(rand.Intn(491) + 10)
}

// randomDelay gives a random wait time.
func randomDelay() time.Duration {
	return time.Duration(rand.Intn(5)+2) * time.Second
}

// isInvalid returns true 10% of the time.
func isInvalid() bool {
	return rand.Intn(10) == 0
}
