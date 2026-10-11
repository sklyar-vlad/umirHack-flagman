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
	analyticshandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/analytics"
	auditloghandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auditlog"
	authhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/auth"
	batchhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/batch"
	cataloghandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/catalog"
	dashboardhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/dashboard"
	dishhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/dish"
	eventhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/events"
	forecasthandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/forecast"
	offershandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/hotoffers"
	inventoryhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/inventory"
	menuhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/menu"
	orderhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/order"
	procurementhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/procurement"
	producthandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/product"
	settingshandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/settings"
	supplyhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/supply"
	userhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/user"
	writeoffhandler "github.com/sklyar-vlad/umirHack-flagman/internal/handler/writeoff"
	"github.com/sklyar-vlad/umirHack-flagman/internal/repository"
	analyticsrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/analytics"
	auditlogrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auditlog"
	authrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/auth"
	batchrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/batch"
	catalogrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/catalog"
	dashboardrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dashboard"
	dishrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/dish"
	eventrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/events"
	expirationrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/expiration"
	forecastrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/forecast"
	offersrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/hotoffers"
	inventoryrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/inventory"
	menurepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/menu"
	orderrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/order"
	procurementrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/procurement"
	productrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/product"
	settingsrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/settings"
	supplyrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/supply"
	userrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/user"
	writeoffrepository "github.com/sklyar-vlad/umirHack-flagman/internal/repository/writeoff"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service"
	analyticsservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/analytics"
	auditlogservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auditlog"
	authservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/auth"
	batchservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/batch"
	catalogservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/catalog"
	dashboardservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/dashboard"
	dishservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/dish"
	eventservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/events"
	expirationservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/expiration"
	forecastservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/forecast"
	offersservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/hotoffers"
	"github.com/sklyar-vlad/umirHack-flagman/internal/service/idempotency"
	inventoryservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/inventory"
	menuservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/menu"
	orderservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/order"
	procurementservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/procurement"
	productservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/product"
	settingsservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/settings"
	supplyservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/supply"
	userservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/user"
	writeoffservice "github.com/sklyar-vlad/umirHack-flagman/internal/service/writeoff"
	"github.com/sklyar-vlad/umirHack-flagman/internal/telemetry"
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
	idem := idempotency.NewService(repo)
	supplies := supplyservice.NewService(supplyrepository.NewRepository(), repo, idem)
	serv := service.NewService(repo, logger, supplies)
	sessionAuth, err := authservice.NewService(authrepository.NewRepository(pool), cfg.Auth.FrontendOrigin)
	if err != nil {
		logger.Fatal("failed configure session authentication", zap.Error(err))
	}
	access := authhandler.NewHandler(sessionAuth)
	hand := handler.NewHandler(supplyhandler.NewHandler(serv), access, logger)
	catalog := catalogservice.NewService(catalogrepository.NewRepository(), repo, repo)
	dishes := dishservice.NewService(dishrepository.NewRepository(), menurepository.NewRepository(), repo, repo)
	settings := settingsservice.NewService(settingsrepository.NewRepository(), repo, repo)
	analytics := analyticsservice.NewService(analyticsrepository.NewRepository(), repo)
	forecast := forecastservice.NewService(forecastrepository.NewRepository(), menurepository.NewRepository(), repo)
	stockMovements := writeoffservice.NewHistoryService(writeoffrepository.NewRepository(), repo)
	writeOff := writeoffservice.NewService(writeoffrepository.NewRepository(), repo, idem)
	auditLog := auditlogservice.NewService(auditlogrepository.NewRepository(), repo)
	products := productservice.NewService(productrepository.NewRepository(), repo)
	inventory := inventoryservice.NewService(inventoryrepository.NewRepository(), repo)
	batches := batchservice.NewService(batchrepository.NewRepository(), repo)
	menu := menuservice.NewService(menurepository.NewRepository(), repo)
	orders := orderservice.NewService(menurepository.NewRepository(), repo)
	orderCreation := orderservice.NewCreateService(orderrepository.NewRepository(), repo, idem)
	orderHistory := orderservice.NewHistoryService(orderrepository.NewRepository(), repo)
	procurement := procurementservice.NewService(
		procurementrepository.NewRepository(),
		menurepository.NewRepository(),
		repo,
	)
	hotOffers := offersservice.NewService(offersrepository.NewRepository(), menurepository.NewRepository(), repo)
	users := userservice.NewService(userrepository.NewRepository(), repo, repo)
	dashboard := dashboardservice.NewService(dashboardrepository.NewRepository(), repo)
	cleanupCtx, cancelCleanup := context.WithCancel(context.Background())
	cleanupDone := make(chan struct{})
	go func() {
		defer close(cleanupDone)
		idem.RunCleanup(cleanupCtx, logger)
	}()
	defer func() {
		cancelCleanup()
		<-cleanupDone
	}()

	expiry := expirationservice.NewService(expirationrepository.NewRepository(), repo, repo, logger)
	expiryWorker := expirationservice.NewWorker(settingsrepository.NewRepository(), repo, expiry, logger)
	expiryCtx, cancelExpiry := context.WithCancel(context.Background())
	expiryDone := make(chan struct{})
	go func() {
		defer close(expiryDone)
		expiryWorker.Run(expiryCtx)
	}()
	defer func() {
		cancelExpiry()
		<-expiryDone
	}()

	const (
		maxBodyBytes      int64 = 1 << 20
		requestsPerSecond       = 10
		burstRequests           = 20
	)

	eventStream := eventservice.NewService(eventrepository.NewRepository(), repo, logger)
	eventsCtx, cancelEvents := context.WithCancel(context.Background())
	eventsDone := make(chan struct{})
	go func() { defer close(eventsDone); eventStream.Run(eventsCtx) }()
	defer func() { cancelEvents(); <-eventsDone }()

	mux := gin.New()
	mux.Use(
		handler.RequestID(),
		telemetry.Default.Middleware(),
		handler.RequestLogger(logger),
		handler.Recovery(logger),
		access.CORS(),
		handler.RateLimit(requestsPerSecond, burstRequests),
		handler.BodyLimit(maxBodyBytes),
	)
	mux.NoRoute(handler.NotFound)

	api := mux.Group("/api/v1", access.RequireRouteAccess)
	api.GET("/ready", handler.Readiness(func(ctx context.Context) error { return database.CheckReady(ctx, pool) }))
	handler.RegisterStopListForecastRoutes(api, forecasthandler.NewHandler(forecast))
	handler.RegisterIngredientShortageAnalyticsRoutes(api, analyticshandler.NewHandler(analytics))
	handler.RegisterWriteOffAnalyticsRoutes(api, analyticshandler.NewHandler(analytics))
	handler.RegisterStockMovementRoutes(api, writeoffhandler.NewHistoryHandler(stockMovements))
	handler.RegisterWriteOffRoutes(api, writeoffhandler.NewHandler(writeOff))
	handler.RegisterAuditLogRoutes(api, auditloghandler.NewHandler(auditLog))
	handler.RegisterSettingsRoutes(api, settingshandler.NewHandler(settings))
	handler.RegisterAuthRoutes(api, access)
	handler.RegisterEventRoutes(api, eventhandler.NewHandler(eventStream, sessionAuth, eventsCtx))
	handler.RegisterUserReadRoutes(api, userhandler.NewHandler(users))
	handler.RegisterUserWriteRoutes(api, userhandler.NewHandler(users))
	handler.RegisterRoutes(api, hand)
	handler.RegisterCatalogRoutes(api, cataloghandler.NewHandler(catalog))
	handler.RegisterDishCreateRoutes(api, dishhandler.NewHandler(dishes))
	handler.RegisterDishUpdateRoutes(api, dishhandler.NewHandler(dishes))
	handler.RegisterDishPreviewRoutes(api, dishhandler.NewHandler(dishes))
	handler.RegisterProductRoutes(api, producthandler.NewHandler(products), access)
	handler.RegisterInventoryRoutes(api, inventoryhandler.NewHandler(inventory), access)
	handler.RegisterBatchRoutes(api, batchhandler.NewHandler(batches), access)
	handler.RegisterMenuRoutes(api, menuhandler.NewHandler(menu), access)
	handler.RegisterOrderCheckRoutes(api, orderhandler.NewHandler(orders), access)
	handler.RegisterOrderCreateRoutes(api, orderhandler.NewCreateHandler(orderCreation), access)
	handler.RegisterOrderHistoryRoutes(api, orderhandler.NewHistoryHandler(orderHistory), access)
	handler.RegisterProcurementRoutes(api, procurementhandler.NewHandler(procurement), access)
	handler.RegisterHotOffersRoutes(api, offershandler.NewHandler(hotOffers), access)
	handler.RegisterDashboardRoutes(api, dashboardhandler.NewHandler(dashboard), access)

	telemetry.Default.RegisterSnapshot(func(ctx context.Context) (float64, float64, float64, error) {
		return database.MetricsSnapshot(ctx, pool)
	})
	root := http.NewServeMux()
	root.Handle("/metrics", telemetry.Default.Handler(cfg.MetricsToken))
	root.Handle("/", mux)
	server := &http.Server{
		Addr:        net.JoinHostPort("", cfg.Server.Port),
		Handler:     root,
		ReadTimeout: time.Duration(cfg.Server.ReadTimeout) * time.Second,
		// SSE replaces the whole-response timeout with renewed per-write deadlines.
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
	cancelEvents()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
		_ = server.Close()
	} else {
		logger.Info("server stopped gracefully")
	}
}
