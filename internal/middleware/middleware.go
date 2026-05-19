// Пакет middleware содержит HTTP-middleware.
// Реализуйте Auth, Logging и Recover самостоятельно.
package middleware

import (
	"context"
	"gopherledger/internal/auth"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

var LogLevel = "info"

// Auth проверяет токен из заголовка Authorization и помещает ID пользователя в контекст.
// Запросы без валидного токена получают ответ 401 Unauthorized.
//
// Что нужно сделать:
//   - прочитать токен из заголовка
//   - проверить токен через пакет auth
//   - поместить ID пользователя в контекст запроса
//   - передать управление следующему handler или вернуть 401
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		userID, err := auth.ValidateToken(token)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), "userID", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// statusRecorder оборачивает http.ResponseWriter для перехвата статус-кода.
// Используйте эту структуру в Logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Logging логирует метод, путь, статус ответа и время выполнения каждого запроса.
//
// Что нужно сделать:
//   - зафиксировать время начала запроса
//   - обернуть w в statusRecorder для перехвата статус-кода
//   - после выполнения handler записать лог
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)
		dur := time.Since(start)
		log.Printf("method=%s path=%s status=%d time=%v", r.Method, r.URL.Path, recorder.status, dur)
	})
}

// Recover перехватывает панику внутри handler, логирует её и возвращает
// клиенту ответ 500 Internal Server Error вместо того, чтобы уронить сервер.
//
// Что нужно сделать:
//   - добавить defer с вызовом recover()
//   - если паника произошла, залогировать её и отдать 500
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			err := recover()
			if err != nil {
				log.Printf("PANIC: %v", err)
				debug.PrintStack()
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
