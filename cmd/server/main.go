// Точка входа сервера. Реализуйте самостоятельно.
//
// Порядок инициализации:
//  1. Загрузить конфигурацию (пакет config)
//  2. Создать хранилище (пакет store)
//  3. Создать сервис (пакет service)
//  4. Запустить воркер начислений в горутине (svc.StartAccrualWorker)
//  5. Создать обработчик и роутер (пакеты handler, router)
//  6. Запустить HTTP-сервер
//  7. Реализовать graceful shutdown по сигналам SIGINT и SIGTERM
package main

import (
	"context"
	"fmt"
	"gopherledger/internal/config"
	"gopherledger/internal/handler"
	"gopherledger/internal/middleware"
	"gopherledger/internal/router"
	"gopherledger/internal/service"
	"gopherledger/internal/store"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("ошибка загрузки конфига: %v", err)
	}

	middleware.LogLevel = cfg.LogLevel

	store := store.New()
	svc := service.New(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go svc.StartAccrualWorker(ctx)

	h := handler.New(svc)
	r := router.New(h)

	address := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
	server := &http.Server{
		Addr:    address,
		Handler: r,
	}

	go func() {
		log.Printf("Сервер запущен на %s:%d", cfg.ServerHost, cfg.ServerPort)
		err = server.ListenAndServe()
		if (err != nil) && (err != http.ErrServerClosed) {
			log.Fatalf("ошибка сервера: %v", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Println("Получен сигнал завершения, начинаем graceful shutdown...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Ошибка при shutdown: %v", err)
		server.Close()
	}

	log.Println("Сервер корректно завершён")

}
