// Пакет auth отвечает за генерацию и проверку токенов аутентификации.
// Токен - это случайная уникальная строка (например, UUID или hex-строка),
// которая однозначно связана с конкретным пользователем.
//
// Внутри пакета нужно хранить соответствие токен -> userID.
// Используйте для этого map с защитой от конкурентного доступа.
// Реализуйте этот пакет самостоятельно.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
)

var (
	mu          sync.Mutex
	tokens      = make(map[string]int64)
	userToToken = make(map[int64]string)
)

// ErrInvalidToken возвращается, если токен не найден или недействителен.
var ErrInvalidToken = errors.New("недействительный токен")

// GenerateToken создаёт новый токен для пользователя с указанным ID
// и сохраняет связь токен -> userID внутри пакета.
func GenerateToken(userID int64) (string, error) {
	mu.Lock()
	defer mu.Unlock()

	prevToken, ok := userToToken[userID]
	if ok {
		delete(tokens, prevToken)
	}

	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)
	tokens[token] = userID
	userToToken[userID] = token

	return token, nil
}

// ValidateToken проверяет токен и возвращает ID пользователя.
// Возвращает ErrInvalidToken если токен не найден.
func ValidateToken(token string) (int64, error) {
	mu.Lock()
	defer mu.Unlock()

	userID, ok := tokens[token]
	if !ok {
		return 0, ErrInvalidToken
	}

	return userID, nil

}
