package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sklyar-vlad/umirHack-flagman/database"
	"github.com/sklyar-vlad/umirHack-flagman/internal/config"
	"github.com/sklyar-vlad/umirHack-flagman/internal/handler"
	supplyhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/supply"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service"
	customLogger "github.com/sklyar-vlad/umirHack-flagman/logger"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.NewConfig()
	if err != nil {
		log.Fatalf("failed create config: %v", err)
	}

	logger, err := customLogger.NewLogger(cfg.Logger)
	if err != nil {
		log.Fatalf("failed create logger: %v", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	pool, err := database.NewPostgres(ctx, cfg.Database)
	cancel()
	if err != nil {
		logger.Fatal("failed connect to the postgres", zap.Error(err))
	}
	defer pool.Close()

	repo := repository.NewRepository(pool, logger)
	serv := service.NewService(repo, logger)
	hand := handler.NewHandler(serv, logger)

	const (
		maxBodyBytes      int64 = 1 << 20
		requestsPerSecond       = 10
		burstRequests           = 20
	)

	mux := gin.New()
	mux.Use(
		handler.RequestID(),
		handler.RequestLogger(logger),
		handler.Recovery(logger),
		handler.RateLimit(requestsPerSecond, burstRequests),
		handler.BodyLimit(maxBodyBytes),
	)
	mux.NoRoute(handler.NotFound)

	api := mux.Group("/api/v1")
	handler.RegisterRoutes(api, hand)
	handler.RegisterSupplyRoutes(api, supplyhandler.NewHandler(serv))

	server := &http.Server{
		Addr:         net.JoinHostPort("", cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeout) * time.Second,
	}

	go func() {
		logger.Info("service started...")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server failed", zap.Error(err))
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	<-stop

	logger.Info("shutdown signal received...")
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	} else {
		logger.Info("server stopped gracefully")
	}
}
