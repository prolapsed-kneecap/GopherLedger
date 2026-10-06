// Пакет store реализует хранилище данных в памяти.
// Используйте отдельные мьютексы для независимых групп данных.
// Реализуйте этот пакет самостоятельно.
package store

import (
	"gopherledger/internal/domain"
	"slices"
	"sync"
	"time"
)

// Store хранит все данные приложения в памяти.
// Добавьте средства защиты конкурентного доступа самостоятельно.
type Store struct {
	mu sync.Mutex
	// users хранит пользователей по их ID
	users map[int64]*domain.User

	// usersByLogin хранит пользователей по логину - для быстрого поиска при авторизации
	usersByLogin map[string]*domain.User

	// orders хранит заказы по номеру заказа
	orders map[string]*domain.Order

	// balances хранит текущий баланс каждого пользователя по его ID
	balances map[int64]*domain.Balance

	// withdrawals хранит историю списаний для каждого пользователя по его ID
	withdrawals map[int64][]*domain.Withdrawal

	// nextID используется для генерации уникальных числовых ID
	nextID int64
}

// New создаёт и возвращает новое пустое хранилище.
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

// CreateUser добавляет нового пользователя.
// Возвращает domain.ErrUserExists если логин уже занят.
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

// GetUserByLogin возвращает пользователя по логину.
// Возвращает domain.ErrUserNotFound если пользователь не найден.
func (s *Store) GetUserByLogin(login string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, ok := s.usersByLogin[login]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

// CreateOrder добавляет новый заказ для пользователя.
// Возвращает domain.ErrOrderOwnedByUser если этот пользователь уже загружал этот номер.
// Возвращает domain.ErrOrderExists если номер принадлежит другому пользователю.
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

// GetUserOrders возвращает все заказы пользователя, сначала новые.
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

// GetOrdersForProcessing возвращает все заказы в статусе NEW или PROCESSING.
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

// UpdateOrderStatus обновляет статус и начисление заказа.
// Если статус PROCESSED и accrual > 0, баланс пользователя пополняется.
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

// GetBalance возвращает баланс пользователя.
func (s *Store) GetBalance(userID int64) (domain.Balance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	balance, ok := s.balances[userID]
	if !ok {
		return domain.Balance{Current: 0, Withdrawn: 0}, nil
	}
	return *balance, nil
}

// Withdraw списывает сумму с баланса и записывает операцию.
// Возвращает domain.ErrInsufficientFunds если баланса не хватает.
// Обе операции должны быть атомарны: либо обе успешны, либо ни одна.
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

// GetWithdrawals возвращает историю списаний пользователя, сначала новые.
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
