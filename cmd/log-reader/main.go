package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/J0es1ick/Scheduler/internal/servicelogs"
)

func setting(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func main() {
	if err := run(); err != nil {
		slog.Error("log reader stopped", "err", err)
		os.Exit(1)
	}
}

func run() error {
	project := os.Getenv("SCHEDULER_LOG_PROJECT")
	if project == "" {
		return errors.New("SCHEDULER_LOG_PROJECT is required")
	}
	store, err := servicelogs.OpenStore(setting("SCHEDULER_LOG_DIRECTORY", "/var/lib/scheduler-logs"))
	if err != nil {
		return err
	}
	defer store.Close()
	if err = store.Purge(time.Now()); err != nil {
		return err
	}
	packets, err := net.ListenPacket("udp", setting("SCHEDULER_LOG_LISTEN", ":15140"))
	if err != nil {
		return err
	}
	defer packets.Close()
	socket := setting("SCHEDULER_LOG_SOCKET", "/run/scheduler-logs/reader.sock")
	if err = os.MkdirAll(filepath.Dir(socket), 0700); err != nil {
		return err
	}
	if info, err := os.Lstat(socket); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return errors.New("log reader path is not a socket")
		}
		if err = os.Remove(socket); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.Chmod(socket, 0600); err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	var workers sync.WaitGroup
	workers.Go(func() {
		buffer := make([]byte, 65536)
		for {
			count, _, err := packets.ReadFrom(buffer)
			if err != nil {
				return
			}
			component, at, text, err := servicelogs.ParseSyslog(buffer[:count], project)
			if err != nil {
				continue
			}
			if err = store.Append(component, at, text); err != nil {
				slog.Error("log storage write failed", "err", err)
			}
		}
	})
	workers.Go(func() {
		timer := time.NewTicker(time.Hour)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				if err := store.Purge(time.Now()); err != nil {
					slog.Error("log retention failed", "err", err)
				}
			}
		}
	})
	_ = store.Append("log-reader", time.Now(), `{"level":"INFO","msg":"log collector started"}`)
	server := &http.Server{Handler: store, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 << 10}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
	case err = <-done:
	}
	cancel()
	packets.Close()
	shutdown, stop := context.WithTimeout(context.Background(), 12*time.Second)
	defer stop()
	if shutdownErr := server.Shutdown(shutdown); shutdownErr != nil {
		server.Close()
	}
	workers.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
