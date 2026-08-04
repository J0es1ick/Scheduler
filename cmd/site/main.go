package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/J0es1ick/Scheduler/internal/config"
	"github.com/J0es1ick/Scheduler/internal/database"
	"github.com/J0es1ick/Scheduler/internal/site"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.InitSiteConfig()
	if err != nil {
		logger.Error("site config init failed", "err", err)
		os.Exit(1)
	}
	db, err := database.NewDatabase(cfg)
	if err != nil {
		logger.Error("site database connect failed", "err", err)
		os.Exit(1)
	}
	defer db.Close()

	siteServer, err := site.NewServer(
		site.NewStore(db.DB),
		cfg.ProjectURL,
		cfg.BotPublicURL,
	)
	if err != nil {
		logger.Error("site server init failed", "err", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:              ":" + cfg.Site.Port,
		Handler:           siteServer.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	rootCtx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()
	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("public site started", "address", httpServer.Addr)
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case <-rootCtx.Done():
		logger.Info("public site shutdown signal received")
	case err = <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("public site failed", "err", err)
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("public site graceful shutdown failed", "err", err)
	}
}
