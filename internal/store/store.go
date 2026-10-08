// Package store saves data in memory.
package store

import (
	"gopherledger/internal/domain"
	"slices"
	"sync"
	"time"
)

// Store keeps all app data in memory.
// It uses a mutex to be safe with goroutines.
type Store struct {
	mu sync.Mutex
	// users maps user ID to User
	users map[int64]*domain.User

	// usersByLogin helps find users quickly by login
	usersByLogin map[string]*domain.User

	// orders maps order number to Order
	orders map[string]*domain.Order

	// balances keeps track of user points
	balances map[int64]*domain.Balance

	// withdrawals keeps the history of spent points
	withdrawals map[int64][]*domain.Withdrawal

	// nextID makes new IDs
	nextID int64
}

// New makes a fresh store.
func New() *Store {
	return &Store{
		users:        make(map[int64]*domain.User),
		usersByLogin: make(map[string]*domain.User),
		orders:       make(map[string]*domain.Order),
		balances:     make(map[int64]*domain.Balance),
		withdrawals:  make(map[int64][]*domain.Withdrawal),
		nextID:       1,
	}
}

// CreateUser saves a new user.
// Returns an error if the login is taken.
func (s *Store) CreateUser(login, passwordHash string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, ok := s.usersByLogin[login]
	if ok {
		return nil, domain.ErrUserExists
	}
	id := s.nextID
	s.nextID++
	user := &domain.User{
		ID:           id,
		Login:        login,
		PasswordHash: passwordHash,
	}
	s.users[id] = user
	s.usersByLogin[login] = user
	return user, nil
}

// GetUserByLogin finds a user by login.
// Returns an error if they don't exist.
func (s *Store) GetUserByLogin(login string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.usersByLogin[login]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

// CreateOrder saves a new order.
// Returns an error if the user already added it.
// Returns an error if someone else added it.
func (s *Store) CreateOrder(userID int64, number string) (*domain.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cur, ok := s.orders[number]
	if ok {
		if cur.UserID == userID {
			return nil, domain.ErrOrderOwnedByUser
		}
		return nil, domain.ErrOrderExists
	}

	id := s.nextID
	s.nextID++

	order := &domain.Order{
		ID:         id,
		Number:     number,
		UserID:     userID,
		Status:     domain.OrderStatusNew,
		Accrual:    0,
		UploadedAt: time.Now(),
	}
	s.orders[number] = order

	return order, nil
}

// GetUserOrders gets all user orders, newest first.
func (s *Store) GetUserOrders(userID int64) ([]domain.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	orders := make([]domain.Order, 0)
	for _, order := range s.orders {
		if order.UserID == userID {
			orders = append(orders, *order)
		}
	}

	slices.SortFunc(orders, func(a, b domain.Order) int {
		if a.UploadedAt.After(b.UploadedAt) {
			return -1
		}
		if a.UploadedAt.Before(b.UploadedAt) {
			return 1
		}
		return 0
	})
	return orders, nil
}

// GetOrdersForProcessing gets orders that need work.
func (s *Store) GetOrdersForProcessing() ([]domain.Order, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	orders := make([]domain.Order, 0)
	for _, order := range s.orders {
		if (order.Status == domain.OrderStatusNew) || (order.Status == domain.OrderStatusProcessing) {
			orders = append(orders, *order)
		}
	}
	return orders, nil
}

// UpdateOrderStatus changes the order status and points.
// If done, it adds points to the user.
func (s *Store) UpdateOrderStatus(number, status string, accrual float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	order, ok := s.orders[number]
	if !ok {
		return nil
	}
	order.Status = status

	if status == domain.OrderStatusProcessed && accrual > 0 {
		order.Accrual = accrual

		balance, ok := s.balances[order.UserID]
		if !ok {
			balance = &domain.Balance{Current: 0, Withdrawn: 0}
			s.balances[order.UserID] = balance
		}
		balance.Current += accrual
	}
	return nil
}

// GetBalance gets the user points.
func (s *Store) GetBalance(userID int64) (domain.Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	balance, ok := s.balances[userID]
	if !ok {
		return domain.Balance{Current: 0, Withdrawn: 0}, nil
	}
	return *balance, nil
}

// Withdraw spends points and saves the record.
// Returns an error if not enough points.
// It does both steps safely together.
func (s *Store) Withdraw(userID int64, orderNumber string, sum float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	balance, ok := s.balances[userID]
	if !ok {
		balance = &domain.Balance{Current: 0, Withdrawn: 0}
		s.balances[userID] = balance
	}
	if balance.Current < sum {
		return domain.ErrInsufficientFunds
	}

	balance.Current -= sum
	balance.Withdrawn += sum

	id := s.nextID
	s.nextID++

	withdrawal := &domain.Withdrawal{
		ID:          id,
		UserID:      userID,
		OrderNumber: orderNumber,
		Sum:         sum,
		ProcessedAt: time.Now(),
	}
	s.withdrawals[userID] = append(s.withdrawals[userID], withdrawal)
	return nil
}

// GetWithdrawals gets the spent points history, newest first.
func (s *Store) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	withdrawals, ok := s.withdrawals[userID]
	if !ok {
		return []domain.Withdrawal{}, nil
	}
	ans := make([]domain.Withdrawal, len(withdrawals))
	for i, w := range withdrawals {
		ans[i] = *w
	}
	slices.SortFunc(ans, func(a, b domain.Withdrawal) int {
		if a.ProcessedAt.After(b.ProcessedAt) {
			return -1
		}
		if a.ProcessedAt.Before(b.ProcessedAt) {
			return 1
		}
		return 0
	})

	return ans, nil

}

func (s *Store) GetStats() (domain.Stats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ordersByStatus := make(map[string]int)
	var totalAccrued float64
	for _, order := range s.orders {
		ordersByStatus[order.Status]++
		if order.Status == domain.OrderStatusProcessed {
			totalAccrued += order.Accrual
		}
	}

	var totalWithdrawn float64
	for _, balance := range s.balances {
		totalWithdrawn += balance.Withdrawn
	}

	return domain.Stats{
		TotalUsers:     len(s.users),
		TotalOrders:    len(s.orders),
		OrdersByStatus: ordersByStatus,
		TotalAccrued:   totalAccrued,
		TotalWithdrawn: totalWithdrawn,
		GeneratedAt:    time.Now().UTC(),
	}, nil
}
