package main

import (
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/gofiber/fiber/v2"
	"github.com/maulanashalihin/laju-go/app/cache"
	"github.com/maulanashalihin/laju-go/app/config"
	"github.com/maulanashalihin/laju-go/app/handlers"
	"github.com/maulanashalihin/laju-go/app/repositories"
	"github.com/maulanashalihin/laju-go/app/services"
	"github.com/maulanashalihin/laju-go/app/session"
	"github.com/maulanashalihin/laju-go/routes"
)

// Version & Commit are injected via -ldflags at build time (see Makefile / Dockerfile).
var (
	Version = "dev"
	Commit  = "none"
)

func main() {
	// ── Config ───────────────────────────────────────────────
	cfg := config.Load()

	slog.Info("starting laju-go",
		"version", Version, "commit", Commit,
		"env", cfg.AppEnv, "port", cfg.AppPort)

	// ── Badger KV ────────────────────────────────────────────
	// Ensure the DB directory exists — Badger requires the dir to be present.
	if err := os.MkdirAll(cfg.DBPath, 0700); err != nil {
		slog.Error("failed to create badger dir", "path", cfg.DBPath, "error", err)
		os.Exit(1)
	}

	opts := badger.DefaultOptions(cfg.DBPath).
		WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opts)
	if err != nil {
		slog.Error("failed to open badger", "path", cfg.DBPath, "error", err)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("badger opened", "path", cfg.DBPath)

	// Background GC: keeps the value log from growing unbounded.
	go runBadgerGC(db)

	// ── Repositories & Cache ────────────────────────────────
	querier := repositories.NewRepository(db)
	sessionCache := cache.NewSessionCache()

	// ── Session store ───────────────────────────────────────
	store := session.New(querier, sessionCache, cfg.SessionTTL)
	store.SetSecure(!cfg.IsDevelopment())

	// ── Services ────────────────────────────────────────────
	assetService := services.NewAssetService(
		filepath.Join("dist", ".vite", "manifest.json"),
		".vite-port",
		cfg.IsDevelopment(),
	)
	inertiaService := services.NewInertiaService(assetService, store)
	authService := services.NewAuthService(querier, services.AuthServiceConfig{
		SessionSecret:      cfg.SessionSecret,
		GoogleClientID:     cfg.GoogleClientID,
		GoogleClientSecret: cfg.GoogleClientSecret,
		GoogleRedirectURL:  cfg.GoogleRedirectURL,
	})
	userService := services.NewUserService(querier)
	mailerService := services.NewMailerService(
		querier,
		cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass,
		cfg.FromEmail, cfg.FromName,
		appURL(cfg),
	)

	// ── Handlers ────────────────────────────────────────────
	publicHandler := handlers.NewPublicHandler(authService, userService, inertiaService, assetService)
	authHandler := handlers.NewAuthHandler(authService, store, inertiaService)
	appHandler := handlers.NewAppHandler(userService, store, inertiaService)
	uploadHandler := handlers.NewUploadHandler(store, userService, "storage/uploads")
	passwordResetHandler := routes.SetupPasswordResetHandler(
		mailerService, userService, store, inertiaService,
	)

	// ── Middlewares ─────────────────────────────────────────
	csrfMiddleware := routes.SetupCSRFMiddleware(cfg.SessionSecret, !cfg.IsDevelopment())

	// ── Fiber app ───────────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName:      "laju-go",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	})

	routes.SetupRoutes(
		app,
		routes.Handlers{
			Public:        publicHandler,
			Auth:          authHandler,
			App:           appHandler,
			Upload:        uploadHandler,
			PasswordReset: passwordResetHandler,
		},
		store, userService, mailerService, csrfMiddleware,
	)

	// ── Start server with graceful shutdown ─────────────────
	go func() {
		addr := ":" + cfg.AppPort
		slog.Info("listening", "addr", addr)
		if err := app.Listen(addr); err != nil {
			slog.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("shutting down...")

	if err := app.ShutdownWithTimeout(15 * time.Second); err != nil {
		slog.Error("forced shutdown", "error", err)
	}
	db.Close() // close immediately so deferred Close is a no-op
	slog.Info("bye")
}

// runBadgerGC periodically reclaims discarded value log space.
// Without this, the value log file grows monotonically under write-heavy loads.
func runBadgerGC(db *badger.DB) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
	again:
		// Run GC until one round reports nothing to reclaim.
		if err := db.RunValueLogGC(0.5); err == nil {
			goto again
		}
	}
}

// appURL returns the canonical app URL for outbound links (email, OAuth, etc.).
// In production this is taken from FRONTEND_URL or APP_URL; in dev it's the
// local Fiber server.
func appURL(cfg *config.Config) string {
	if cfg.AppEnv == "production" {
		if cfg.FrontendURL != "" && cfg.FrontendURL != "http://localhost:5173" {
			return cfg.FrontendURL
		}
	}
	return "http://localhost:" + cfg.AppPort
}
