// Пакет handler содержит HTTP-обработчики.
//
// Взаимодействие с бизнес-логикой осуществляется через интерфейс.
// Определите этот интерфейс здесь, по месту использования.
// Реализуйте все обработчики самостоятельно.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"gopherledger/internal/domain"
)

type Service interface {
	RegisterUser(login, password string) (string, error)
	LoginUser(login, password string) (string, error)
	CreateOrder(userID int64, number string) (*domain.Order, error)
	GetUserOrders(userID int64) ([]domain.Order, error)
	GetBalance(userID int64) (domain.Balance, error)
	Withdraw(userID int64, orderNumber string, sum float64) error
	GetWithdrawals(userID int64) ([]domain.Withdrawal, error)
	GetStats() (domain.Stats, error)
}

// Handler хранит зависимость от бизнес-логики.
// Замените interface{} на свой интерфейс.
type Handler struct {
	svc Service
}

// New создаёт Handler.
func New(svc Service) *Handler {
	return &Handler{svc: svc}
}

type authRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type withdrawRequest struct {
	Order string  `json:"order"`
	Sum   float64 `json:"sum"`
}

type orderResponse struct {
	Number     string    `json:"number"`
	Status     string    `json:"status"`
	Accrual    *float64  `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at"`
}

type balanceResponse struct {
	Current   float64 `json:"current"`
	Withdrawn float64 `json:"withdrawn"`
}

type withdrawalResponse struct {
	Order       string    `json:"order"`
	Sum         float64   `json:"sum"`
	ProcessedAt time.Time `json:"processed_at"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ---------------------------------------------------------------------------
// Вспомогательные функции для ответов - предоставлены
// ---------------------------------------------------------------------------

// writeError записывает JSON-ответ с ошибкой.
// Клиент видит только userMsg. Внутренние детали пишутся только в лог.
// Прочитайте ТЗ и создайте структуру тела ответа самостоятельно.
func writeError(w http.ResponseWriter, status int, code, userMsg string, internalErr error) {
	if internalErr != nil {
		log.Printf("ошибка code=%s status=%d: %v", code, status, internalErr)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// TODO: создайте структуру ответа и сериализуйте её
	json.NewEncoder(w).Encode(errorResponse{Code: code, Message: userMsg})
}

// writeJSON записывает успешный JSON-ответ.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Обработчики - реализуйте самостоятельно
// ---------------------------------------------------------------------------

// Register обрабатывает POST /api/user/register.
// При успехе: 200 OK, заголовок Authorization с токеном.
// При дублировании логина: 409 Conflict.
// При некорректных данных: 400 Bad Request.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неверный формат запроса", err)
		return
	}
	defer r.Body.Close()

	err = json.Unmarshal(body, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неверный формат запроса", err)
		return
	}
	if req.Login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "логин и пароль обязательны", nil)
		return
	}
	token, err := h.svc.RegisterUser(req.Login, req.Password)
	if err != nil {
		if err == domain.ErrUserExists {
			writeError(w, http.StatusConflict, "USER_EXISTS", "пользователь уже существует", err)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "внутренняя ошибка", err)
		}
		return
	}

	w.Header().Set("Authorization", token)
	w.WriteHeader(http.StatusOK)
}

// Login обрабатывает POST /api/user/login.
// При успехе: 200 OK, заголовок Authorization с токеном.
// При неверных данных: 401 Unauthorized.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req authRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неверный формат запроса", err)
		return
	}
	defer r.Body.Close()
	err = json.Unmarshal(body, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неверный формат запроса", err)
		return
	}
	if req.Login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "логин и пароль обязательны", nil)
		return
	}
	token, err := h.svc.LoginUser(req.Login, req.Password)
	if err != nil {
		if (err == domain.ErrUserNotFound) || (err == domain.ErrInvalidPassword) {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "неверный логин или пароль", err)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "внутренняя ошибка", err)
		}
		return
	}
	w.Header().Set("Authorization", token)
	w.WriteHeader(http.StatusOK)
}

// CreateOrder обрабатывает POST /api/user/orders.
// Тело запроса: номер заказа в виде обычного текста.
// 202 Accepted  - новый заказ принят в обработку.
// 200 OK        - заказ уже загружен этим пользователем.
// 409 Conflict  - заказ принадлежит другому пользователю.
// 422 Unprocessable Entity - номер не прошёл проверку Луна.
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "не авторизован", nil)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неверный формат запроса", err)
		return
	}
	defer r.Body.Close()
	number := strings.TrimSpace(string(body))

	order, err := h.svc.CreateOrder(userID, number)
	if err != nil {
		if err == domain.ErrInvalidOrder {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_ORDER", "неверный номер заказа", err)
		} else if err == domain.ErrOrderOwnedByUser {
			w.WriteHeader(http.StatusOK)
		} else if err == domain.ErrOrderExists {
			writeError(w, http.StatusConflict, "ORDER_EXISTS", "заказ принадлежит другому пользователю", err)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "внутренняя ошибка", err)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, order)
}

// GetOrders обрабатывает GET /api/user/orders.
// 200 OK с JSON-массивом заказов или 204 No Content если заказов нет.
func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "не авторизован", nil)
		return
	}
	orders, err := h.svc.GetUserOrders(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "внутренняя ошибка", nil)
		return
	}
	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	response := make([]orderResponse, len(orders))
	for i, order := range orders {
		var accrual *float64
		if order.Status == domain.OrderStatusProcessed {
			a := order.Accrual
			accrual = &a
		}
		response[i] = orderResponse{
			Number:     order.Number,
			Status:     order.Status,
			Accrual:    accrual,
			UploadedAt: order.UploadedAt,
		}
	}

	writeJSON(w, http.StatusOK, response)

}

// GetBalance обрабатывает GET /api/user/balance.
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "не авторизован", nil)
		return
	}
	balance, err := h.svc.GetBalance(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "внутренняя ошибка", err)
		return
	}
	response := balanceResponse{
		Current:   balance.Current,
		Withdrawn: balance.Withdrawn,
	}
	writeJSON(w, http.StatusOK, response)
}

// Withdraw обрабатывает POST /api/user/balance/withdraw.
// 200 OK при успехе.
// 402 Payment Required при нехватке баллов.
// 422 Unprocessable Entity при неверном номере заказа.
func (h *Handler) Withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "не авторизован", nil)
		return
	}
	var req withdrawRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неверный формат запроса", err)
		return
	}
	defer r.Body.Close()

	err = json.Unmarshal(body, &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "неверный формат запроса", err)
		return
	}

	err = h.svc.Withdraw(userID, req.Order, req.Sum)
	if err != nil {
		if err == domain.ErrInvalidOrder {
			writeError(w, http.StatusUnprocessableEntity, "INVALID_ORDER", "неверный номер заказа", err)
		} else if err == domain.ErrInsufficientFunds {
			writeError(w, http.StatusPaymentRequired, "INSUFFICIENT_FUNDS", "недостаточно баллов", err)
		} else {
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "внутренняя ошибка", err)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

// GetWithdrawals обрабатывает GET /api/user/withdrawals.
// 200 OK с массивом или 204 No Content если списаний нет.
func (h *Handler) GetWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "не авторизован", nil)
		return
	}
	withdrawals, err := h.svc.GetWithdrawals(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "внутренняя ошибка", err)
		return
	}
	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	response := make([]withdrawalResponse, len(withdrawals))
	for i, withdrawal := range withdrawals {
		response[i] = withdrawalResponse{
			Order:       withdrawal.OrderNumber,
			Sum:         withdrawal.Sum,
			ProcessedAt: withdrawal.ProcessedAt,
		}
	}
	writeJSON(w, http.StatusOK, response)
}

// ExportStats обрабатывает POST /api/stats/export.
// Собирает статистику системы и записывает её в текстовый файл stats.txt
// в корне проекта. Возвращает 200 OK при успехе.
//
// Файл должен содержать:
//   - общее число зарегистрированных пользователей
//   - общее число заказов и их распределение по статусам
//   - суммарное количество начисленных баллов
//   - суммарное количество списанных баллов
//   - время генерации отчёта
//
// Для работы с файлами используйте пакет os (неделя 8).
func (h *Handler) ExportStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "внутренняя ошибка", err)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Report generated at: %s\n", stats.GeneratedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Total registered users: %d\n", stats.TotalUsers))
	sb.WriteString(fmt.Sprintf("Total orders: %d\n", stats.TotalOrders))
	sb.WriteString("Orders by status:\n")
	sb.WriteString(fmt.Sprintf("  - NEW: %d\n", stats.OrdersByStatus[domain.OrderStatusNew]))
	sb.WriteString(fmt.Sprintf("  - PROCESSING: %d\n", stats.OrdersByStatus[domain.OrderStatusProcessing]))
	sb.WriteString(fmt.Sprintf("  - PROCESSED: %d\n", stats.OrdersByStatus[domain.OrderStatusProcessed]))
	sb.WriteString(fmt.Sprintf("  - INVALID: %d\n", stats.OrdersByStatus[domain.OrderStatusInvalid]))
	sb.WriteString(fmt.Sprintf("Total accrued points: %.2f\n", stats.TotalAccrued))
	sb.WriteString(fmt.Sprintf("Total withdrawn points: %.2f\n", stats.TotalWithdrawn))

	if err := os.WriteFile("stats.txt", []byte(sb.String()), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "ошибка записи файла", err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------------------
// Вспомогательная функция для работы с контекстом - предоставлена
// ---------------------------------------------------------------------------

type contextKey string

const CtxKeyUserID contextKey = "userID"

// UserIDFromContext извлекает ID аутентифицированного пользователя из контекста.
// Возвращает 0, false если значение отсутствует.
func UserIDFromContext(ctx context.Context) (int64, bool) {
	userID, ok := ctx.Value(CtxKeyUserID).(int64)
	return userID, ok
}
