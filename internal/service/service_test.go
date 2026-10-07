package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"gopherledger/internal/domain"
)

type fakeStore struct {
	users       map[string]*domain.User
	orders      map[string]*domain.Order
	balances    map[int64]domain.Balance
	withdrawals map[int64][]domain.Withdrawal
	nextID      int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:       make(map[string]*domain.User),
		orders:      make(map[string]*domain.Order),
		balances:    make(map[int64]domain.Balance),
		withdrawals: make(map[int64][]domain.Withdrawal),
		nextID:      1,
	}
}

func (f *fakeStore) CreateUser(login, passwordHash string) (*domain.User, error) {
	if _, exists := f.users[login]; exists {
		return nil, domain.ErrUserExists
	}
	u := &domain.User{
		ID:           f.nextID,
		Login:        login,
		PasswordHash: passwordHash,
	}
	f.nextID++
	f.users[login] = u
	return u, nil
}

func (f *fakeStore) GetUserByLogin(login string) (*domain.User, error) {
	u, ok := f.users[login]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (f *fakeStore) CreateOrder(userID int64, number string) (*domain.Order, error) {
	if existing, ok := f.orders[number]; ok {
		if existing.UserID == userID {
			return nil, domain.ErrOrderOwnedByUser
		}
		return nil, domain.ErrOrderExists
	}
	o := &domain.Order{
		ID:         f.nextID,
		UserID:     userID,
		Number:     number,
		Status:     domain.OrderStatusNew,
		UploadedAt: time.Now(),
	}
	f.nextID++
	f.orders[number] = o
	return o, nil
}

func (f *fakeStore) GetUserOrders(userID int64) ([]domain.Order, error) {
	var res []domain.Order
	for _, o := range f.orders {
		if o.UserID == userID {
			res = append(res, *o)
		}
	}
	return res, nil
}

func (f *fakeStore) GetOrdersForProcessing() ([]domain.Order, error) {
	return nil, nil
}

func (f *fakeStore) UpdateOrderStatus(number, status string, accrual float64) error {
	return nil
}

func (f *fakeStore) GetBalance(userID int64) (domain.Balance, error) {
	return f.balances[userID], nil
}

func (f *fakeStore) Withdraw(userID int64, orderNumber string, sum float64) error {
	b := f.balances[userID]
	if b.Current < sum {
		return domain.ErrInsufficientFunds
	}
	b.Current -= sum
	b.Withdrawn += sum
	f.balances[userID] = b
	f.withdrawals[userID] = append(f.withdrawals[userID], domain.Withdrawal{
		ID:          f.nextID,
		UserID:      userID,
		OrderNumber: orderNumber,
		Sum:         sum,
		ProcessedAt: time.Now(),
	})
	f.nextID++
	return nil
}

func (f *fakeStore) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	return f.withdrawals[userID], nil
}

func (f *fakeStore) GetStats() (domain.Stats, error) {
	return domain.Stats{
		TotalOrders:    len(f.orders),
		OrdersByStatus: make(map[string]int),
		TotalAccrued:   0,
		TotalWithdrawn: 0,
	}, nil
}

func TestRegisterUser(t *testing.T) {
	tests := []struct {
		name      string
		login     string
		password  string
		setup     func(st *fakeStore)
		wantErr   error
		wantToken bool
	}{
		{
			name:      "successful registration",
			login:     "user1",
			password:  "password123",
			setup:     func(st *fakeStore) {},
			wantErr:   nil,
			wantToken: true,
		},
		{
			name:     "duplicate login",
			login:    "user1",
			password: "password123",
			setup: func(st *fakeStore) {
				hash := sha256.Sum256([]byte("password123"))
				_, _ = st.CreateUser("user1", hex.EncodeToString(hash[:]))
			},
			wantErr:   domain.ErrUserExists,
			wantToken: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newFakeStore()
			tt.setup(st)
			svc := New(st)

			token, err := svc.RegisterUser(tt.login, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantToken && token == "" {
				t.Fatalf("expected non-empty token")
			}
		})
	}
}

func TestLoginUser(t *testing.T) {
	passHash := func(p string) string {
		h := sha256.Sum256([]byte(p))
		return hex.EncodeToString(h[:])
	}

	tests := []struct {
		name      string
		login     string
		password  string
		setup     func(st *fakeStore)
		wantErr   error
		wantToken bool
	}{
		{
			name:     "successful login",
			login:    "user1",
			password: "correctpassword",
			setup: func(st *fakeStore) {
				_, _ = st.CreateUser("user1", passHash("correctpassword"))
			},
			wantErr:   nil,
			wantToken: true,
		},
		{
			name:     "invalid password",
			login:    "user1",
			password: "wrongpassword",
			setup: func(st *fakeStore) {
				_, _ = st.CreateUser("user1", passHash("correctpassword"))
			},
			wantErr:   domain.ErrInvalidPassword,
			wantToken: false,
		},
		{
			name:      "user not found",
			login:     "unknown",
			password:  "password",
			setup:     func(st *fakeStore) {},
			wantErr:   domain.ErrUserNotFound,
			wantToken: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newFakeStore()
			tt.setup(st)
			svc := New(st)

			token, err := svc.LoginUser(tt.login, tt.password)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantToken && token == "" {
				t.Fatalf("expected non-empty token")
			}
		})
	}
}

func TestCreateOrder(t *testing.T) {
	tests := []struct {
		name    string
		userID  int64
		number  string
		setup   func(st *fakeStore)
		wantErr error
	}{
		{
			name:    "successful order creation",
			userID:  1,
			number:  "79927398713",
			setup:   func(st *fakeStore) {},
			wantErr: nil,
		},
		{
			name:    "invalid luhn number",
			userID:  1,
			number:  "12345",
			setup:   func(st *fakeStore) {},
			wantErr: domain.ErrInvalidOrder,
		},
		{
			name:   "order uploaded by same user",
			userID: 1,
			number: "79927398713",
			setup: func(st *fakeStore) {
				_, _ = st.CreateOrder(1, "79927398713")
			},
			wantErr: domain.ErrOrderOwnedByUser,
		},
		{
			name:   "order uploaded by another user",
			userID: 2,
			number: "79927398713",
			setup: func(st *fakeStore) {
				_, _ = st.CreateOrder(1, "79927398713")
			},
			wantErr: domain.ErrOrderExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newFakeStore()
			tt.setup(st)
			svc := New(st)

			order, err := svc.CreateOrder(tt.userID, tt.number)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if tt.wantErr == nil && (order == nil || order.Number != tt.number) {
				t.Fatalf("unexpected order result: %v", order)
			}
		})
	}
}

func TestWithdraw(t *testing.T) {
	tests := []struct {
		name    string
		userID  int64
		order   string
		sum     float64
		setup   func(st *fakeStore)
		wantErr error
	}{
		{
			name:   "successful withdrawal",
			userID: 1,
			order:  "79927398713",
			sum:    200,
			setup: func(st *fakeStore) {
				st.balances[1] = domain.Balance{Current: 500, Withdrawn: 0}
			},
			wantErr: nil,
		},
		{
			name:   "insufficient funds",
			userID: 1,
			order:  "79927398713",
			sum:    600,
			setup: func(st *fakeStore) {
				st.balances[1] = domain.Balance{Current: 500, Withdrawn: 0}
			},
			wantErr: domain.ErrInsufficientFunds,
		},
		{
			name:   "invalid luhn number",
			userID: 1,
			order:  "12345",
			sum:    50,
			setup: func(st *fakeStore) {
				st.balances[1] = domain.Balance{Current: 500, Withdrawn: 0}
			},
			wantErr: domain.ErrInvalidOrder,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := newFakeStore()
			tt.setup(st)
			svc := New(st)

			err := svc.Withdraw(tt.userID, tt.order, tt.sum)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestServiceQueries(t *testing.T) {
	st := newFakeStore()
	svc := New(st)

	_, _ = st.CreateOrder(1, "79927398713")
	st.balances[1] = domain.Balance{Current: 100, Withdrawn: 50}
	_ = st.Withdraw(1, "79927398713", 20)

	orders, err := svc.GetUserOrders(1)
	if err != nil || len(orders) != 1 {
		t.Fatalf("GetUserOrders failed: %v", err)
	}

	balance, err := svc.GetBalance(1)
	if err != nil || balance.Current != 80 || balance.Withdrawn != 70 {
		t.Fatalf("GetBalance failed: %v, balance: %+v", err, balance)
	}

	withdrawals, err := svc.GetWithdrawals(1)
	if err != nil || len(withdrawals) != 1 {
		t.Fatalf("GetWithdrawals failed: %v", err)
	}

	stats, err := svc.GetStats()
	if err != nil || stats.TotalOrders != 1 {
		t.Fatalf("GetStats failed: %v", err)
	}
}