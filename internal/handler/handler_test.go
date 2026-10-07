package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"gopherledger/internal/domain"
)

type fakeService struct {
	registerUserFn   func(login, password string) (string, error)
	loginUserFn      func(login, password string) (string, error)
	createOrderFn    func(userID int64, number string) (*domain.Order, error)
	getUserOrdersFn  func(userID int64) ([]domain.Order, error)
	getBalanceFn     func(userID int64) (domain.Balance, error)
	withdrawFn       func(userID int64, orderNumber string, sum float64) error
	getWithdrawalsFn func(userID int64) ([]domain.Withdrawal, error)
	getStatsFn       func() (domain.Stats, error)
}

func (f *fakeService) RegisterUser(login, password string) (string, error) {
	if f.registerUserFn != nil {
		return f.registerUserFn(login, password)
	}
	return "", nil
}

func (f *fakeService) LoginUser(login, password string) (string, error) {
	if f.loginUserFn != nil {
		return f.loginUserFn(login, password)
	}
	return "", nil
}

func (f *fakeService) CreateOrder(userID int64, number string) (*domain.Order, error) {
	if f.createOrderFn != nil {
		return f.createOrderFn(userID, number)
	}
	return nil, nil
}

func (f *fakeService) GetUserOrders(userID int64) ([]domain.Order, error) {
	if f.getUserOrdersFn != nil {
		return f.getUserOrdersFn(userID)
	}
	return nil, nil
}

func (f *fakeService) GetBalance(userID int64) (domain.Balance, error) {
	if f.getBalanceFn != nil {
		return f.getBalanceFn(userID)
	}
	return domain.Balance{}, nil
}

func (f *fakeService) Withdraw(userID int64, orderNumber string, sum float64) error {
	if f.withdrawFn != nil {
		return f.withdrawFn(userID, orderNumber, sum)
	}
	return nil
}

func (f *fakeService) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	if f.getWithdrawalsFn != nil {
		return f.getWithdrawalsFn(userID)
	}
	return nil, nil
}

func (f *fakeService) GetStats() (domain.Stats, error) {
	if f.getStatsFn != nil {
		return f.getStatsFn()
	}
	return domain.Stats{}, nil
}

func ctxWithUser(r *http.Request, userID int64) *http.Request {
	ctx := context.WithValue(r.Context(), CtxKeyUserID, userID)
	return r.WithContext(ctx)
}

func TestRegister(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		mockSvc      *fakeService
		expectedCode int
	}{
		{
			name: "success",
			body: `{"login":"user","password":"pwd"}`,
			mockSvc: &fakeService{
				registerUserFn: func(login, password string) (string, error) {
					return "token", nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "duplicate login",
			body: `{"login":"user","password":"pwd"}`,
			mockSvc: &fakeService{
				registerUserFn: func(login, password string) (string, error) {
					return "", domain.ErrUserExists
				},
			},
			expectedCode: http.StatusConflict,
		},
		{
			name:         "empty fields",
			body:         `{"login":"","password":""}`,
			mockSvc:      &fakeService{},
			expectedCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(tt.mockSvc)
			req := httptest.NewRequest(http.MethodPost, "/api/user/register", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.Register(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestLogin(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		mockSvc      *fakeService
		expectedCode int
	}{
		{
			name: "success",
			body: `{"login":"user","password":"pwd"}`,
			mockSvc: &fakeService{
				loginUserFn: func(login, password string) (string, error) {
					return "token", nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "invalid credentials",
			body: `{"login":"user","password":"wrong"}`,
			mockSvc: &fakeService{
				loginUserFn: func(login, password string) (string, error) {
					return "", domain.ErrInvalidPassword
				},
			},
			expectedCode: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(tt.mockSvc)
			req := httptest.NewRequest(http.MethodPost, "/api/user/login", strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			h.Login(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestCreateOrder(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		mockSvc      *fakeService
		expectedCode int
	}{
		{
			name: "success",
			body: "12345678903",
			mockSvc: &fakeService{
				createOrderFn: func(userID int64, number string) (*domain.Order, error) {
					return &domain.Order{}, nil
				},
			},
			expectedCode: http.StatusAccepted,
		},
		{
			name: "invalid luhn",
			body: "111",
			mockSvc: &fakeService{
				createOrderFn: func(userID int64, number string) (*domain.Order, error) {
					return nil, domain.ErrInvalidOrder
				},
			},
			expectedCode: http.StatusUnprocessableEntity,
		},
		{
			name: "conflict",
			body: "12345678903",
			mockSvc: &fakeService{
				createOrderFn: func(userID int64, number string) (*domain.Order, error) {
					return nil, domain.ErrOrderExists
				},
			},
			expectedCode: http.StatusConflict,
		},
		{
			name: "already uploaded by user",
			body: "12345678903",
			mockSvc: &fakeService{
				createOrderFn: func(userID int64, number string) (*domain.Order, error) {
					return nil, domain.ErrOrderOwnedByUser
				},
			},
			expectedCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(tt.mockSvc)
			req := httptest.NewRequest(http.MethodPost, "/api/user/orders", strings.NewReader(tt.body))
			req = ctxWithUser(req, 1)
			w := httptest.NewRecorder()
			h.CreateOrder(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestGetOrders(t *testing.T) {
	tests := []struct {
		name         string
		mockSvc      *fakeService
		expectedCode int
	}{
		{
			name: "with orders",
			mockSvc: &fakeService{
				getUserOrdersFn: func(userID int64) ([]domain.Order, error) {
					return []domain.Order{{Number: "123"}}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "no orders",
			mockSvc: &fakeService{
				getUserOrdersFn: func(userID int64) ([]domain.Order, error) {
					return []domain.Order{}, nil
				},
			},
			expectedCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(tt.mockSvc)
			req := httptest.NewRequest(http.MethodGet, "/api/user/orders", nil)
			req = ctxWithUser(req, 1)
			w := httptest.NewRecorder()
			h.GetOrders(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestGetBalance(t *testing.T) {
	mock := &fakeService{
		getBalanceFn: func(userID int64) (domain.Balance, error) {
			return domain.Balance{Current: 100, Withdrawn: 50}, nil
		},
	}
	h := New(mock)
	req := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
	req = ctxWithUser(req, 1)
	w := httptest.NewRecorder()
	h.GetBalance(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestWithdraw(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		mockSvc      *fakeService
		expectedCode int
	}{
		{
			name: "success",
			body: `{"order":"123","sum":10}`,
			mockSvc: &fakeService{
				withdrawFn: func(userID int64, orderNumber string, sum float64) error {
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "insufficient funds",
			body: `{"order":"123","sum":1000}`,
			mockSvc: &fakeService{
				withdrawFn: func(userID int64, orderNumber string, sum float64) error {
					return domain.ErrInsufficientFunds
				},
			},
			expectedCode: http.StatusPaymentRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(tt.mockSvc)
			req := httptest.NewRequest(http.MethodPost, "/api/user/balance/withdraw", strings.NewReader(tt.body))
			req = ctxWithUser(req, 1)
			w := httptest.NewRecorder()
			h.Withdraw(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestGetWithdrawals(t *testing.T) {
	tests := []struct {
		name         string
		mockSvc      *fakeService
		expectedCode int
	}{
		{
			name: "with records",
			mockSvc: &fakeService{
				getWithdrawalsFn: func(userID int64) ([]domain.Withdrawal, error) {
					return []domain.Withdrawal{{OrderNumber: "123", Sum: 10}}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "no records",
			mockSvc: &fakeService{
				getWithdrawalsFn: func(userID int64) ([]domain.Withdrawal, error) {
					return []domain.Withdrawal{}, nil
				},
			},
			expectedCode: http.StatusNoContent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := New(tt.mockSvc)
			req := httptest.NewRequest(http.MethodGet, "/api/user/withdrawals", nil)
			req = ctxWithUser(req, 1)
			w := httptest.NewRecorder()
			h.GetWithdrawals(w, req)

			if w.Code != tt.expectedCode {
				t.Errorf("expected %d, got %d", tt.expectedCode, w.Code)
			}
		})
	}
}

func TestExportStats(t *testing.T) {
	mock := &fakeService{
		getStatsFn: func() (domain.Stats, error) {
			return domain.Stats{
				TotalUsers:  10,
				TotalOrders: 20,
				OrdersByStatus: map[string]int{
					domain.OrderStatusNew: 5,
				},
				TotalAccrued:   100.5,
				TotalWithdrawn: 50.2,
				GeneratedAt:    time.Now(),
			}, nil
		},
	}
	h := New(mock)
	req := httptest.NewRequest(http.MethodPost, "/api/stats/export", nil)
	w := httptest.NewRecorder()
	h.ExportStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	_, err := os.Stat("stats.txt")
	if os.IsNotExist(err) {
		t.Fatalf("expected stats.txt to be created")
	}
	_ = os.Remove("stats.txt")
}