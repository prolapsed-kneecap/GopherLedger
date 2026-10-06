// Пакет service содержит бизнес-логику приложения.
//
// Взаимодействие с хранилищем осуществляется через интерфейс.
// Определите этот интерфейс здесь, по месту использования.
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

// Service реализует бизнес-логику приложения.
// Замените поле repo в структуре на свой интерфейс.
//
// processingOrders хранит номера заказов, которые сейчас обрабатываются воркером.
// Защитите конкурентный доступ к этому полю самостоятельно.

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

// New создаёт Service.
func New(repo Store) *Service {
	return &Service{
		repo:             repo,
		processingOrders: make(map[string]bool),
	}
}

// ---------------------------------------------------------------------------
// Методы бизнес-логики - реализуйте самостоятельно
// ---------------------------------------------------------------------------

// RegisterUser регистрирует нового пользователя и возвращает токен аутентификации.
// Хешируйте пароль перед сохранением с помощью crypto/sha256.
func (s *Service) RegisterUser(login, password string) (string, error) {
	hash := sha256.Sum256([]byte(password))
	passwordHash := hex.EncodeToString(hash[:])

	user, err := s.repo.CreateUser(login, passwordHash)

	if err != nil {
		return "", err
	}

	return auth.GenerateToken(user.ID)
}

// LoginUser проверяет учётные данные и возвращает токен аутентификации.
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

// CreateOrder проверяет номер заказа по алгоритму Луна и сохраняет заказ.
func (s *Service) CreateOrder(userID int64, number string) (*domain.Order, error) {
	if !validateLuhn(number) {
		return nil, domain.ErrInvalidOrder
	}
	return s.repo.CreateOrder(userID, number)
}

// GetUserOrders возвращает все заказы пользователя.
func (s *Service) GetUserOrders(userID int64) ([]domain.Order, error) {
	return s.repo.GetUserOrders(userID)
}

// GetBalance возвращает текущий баланс пользователя.
func (s *Service) GetBalance(userID int64) (domain.Balance, error) {
	return s.repo.GetBalance(userID)
}

// Withdraw проверяет номер заказа по алгоритму Луна и списывает сумму с баланса.
func (s *Service) Withdraw(userID int64, orderNumber string, sum float64) error {
	if !validateLuhn(orderNumber) {
		return domain.ErrInvalidOrder
	}
	return s.repo.Withdraw(userID, orderNumber, sum)
}

// GetWithdrawals возвращает историю списаний пользователя.
func (s *Service) GetWithdrawals(userID int64) ([]domain.Withdrawal, error) {
	return s.repo.GetWithdrawals(userID)
}

func (s *Service) GetStats() (domain.Stats, error) {
	return s.repo.GetStats()
}

// validateLuhn проверяет контрольную сумму номера заказа по алгоритму Луна.
// Вызывается при загрузке заказа и при списании баллов.
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
// Воркер начислений
//
// StartAccrualWorker предоставлен. Реализуйте processAllPendingOrders
// и processOrder самостоятельно.
//
// Это самая интересная часть проекта: конкурентная обработка заказов.
// Подумайте, как защитить доступ к processingOrders из нескольких горутин.
// ---------------------------------------------------------------------------

// StartAccrualWorker запускает фоновый цикл, который каждые 3 секунды
// передаёт необработанные заказы в processAllPendingOrders.
// Останавливается при отмене ctx.
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

// processAllPendingOrders получает заказы для обработки и запускает горутины.
// Реализуйте самостоятельно.
func (s *Service) processAllPendingOrders(ctx context.Context) {
	// TODO: замените interface{} на свой интерфейс и раскомментируйте

	orders, err := s.repo.GetOrdersForProcessing()
	if err != nil {
		log.Printf("воркер: ошибка получения заказов: %v", err)
		return
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	// TODO: итерируйтесь по заказам, пропускайте те что уже в обработке,
	// для остальных запускайте s.processOrder через g.Go

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

// processOrder обрабатывает один заказ. Реализуйте самостоятельно.
// Используйте вспомогательные функции ниже для генерации случайных значений.
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
// Вспомогательные функции - предоставлены
// ---------------------------------------------------------------------------

// randomAccrual возвращает случайное начисление от 10 до 500 баллов.
func randomAccrual() float64 {
	return float64(rand.Intn(491) + 10)
}

// randomDelay возвращает случайную задержку от 2 до 6 секунд.
func randomDelay() time.Duration {
	return time.Duration(rand.Intn(5)+2) * time.Second
}

// isInvalid возвращает true примерно в 10% случаев.
func isInvalid() bool {
	return rand.Intn(10) == 0
}
