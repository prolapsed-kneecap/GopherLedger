// Package auth makes and checks user tokens.
// A token is a random string for each user.
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

// ErrInvalidToken means the token is bad.
var ErrInvalidToken = errors.New("недействительный токен")

// GenerateToken makes a new token for the user.
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

// ValidateToken checks if the token is good and returns the user ID.
func ValidateToken(token string) (int64, error) {
	mu.Lock()
	defer mu.Unlock()

	userID, ok := tokens[token]
	if !ok {
		return 0, ErrInvalidToken
	}

	return userID, nil

}
