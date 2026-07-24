// Command wormhole is a tiny, dependency-free forward HTTP/HTTPS proxy
// designed to be quick to deploy: a single static binary or a small container.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/zollo/wormhole/proxy"
)

// version is stamped at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "wormhole:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := parseConfig()

	if cfg.showVersion {
		fmt.Println("wormhole", version)
		return nil
	}

	level := slog.LevelInfo
	if cfg.verbose {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	handler := proxy.New(proxy.Options{
		Username:    cfg.username,
		Password:    cfg.password,
		DialTimeout: cfg.dialTimeout,
		Logger:      logger,
	})

	server := &http.Server{
		Addr:         cfg.addr,
		Handler:      handler,
		ReadTimeout:  cfg.readTimeout,
		WriteTimeout: 0, // must stay 0 so CONNECT tunnels are not cut off.
		IdleTimeout:  cfg.idleTimeout,
	}

	authMode := "open (no authentication)"
	if cfg.username != "" || cfg.password != "" {
		authMode = "basic auth enabled"
	}
	logger.Info("wormhole starting",
		"version", version,
		"addr", cfg.addr,
		"auth", authMode,
	)

	// Shut down gracefully on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

// config holds resolved runtime settings.
type config struct {
	addr        string
	username    string
	password    string
	dialTimeout time.Duration
	readTimeout time.Duration
	idleTimeout time.Duration
	verbose     bool
	showVersion bool
}

// parseConfig resolves settings from flags, falling back to WORMHOLE_*
// environment variables so the proxy is easy to configure in a container.
func parseConfig() config {
	var cfg config

	flag.StringVar(&cfg.addr, "addr", envStr("WORMHOLE_ADDR", ":8080"),
		"address to listen on (host:port)")
	flag.StringVar(&cfg.username, "user", envStr("WORMHOLE_USER", ""),
		"username for proxy basic auth (empty disables auth)")
	flag.StringVar(&cfg.password, "pass", envStr("WORMHOLE_PASS", ""),
		"password for proxy basic auth")
	flag.DurationVar(&cfg.dialTimeout, "dial-timeout", envDur("WORMHOLE_DIAL_TIMEOUT", 10*time.Second),
		"timeout for connecting to origin servers")
	flag.DurationVar(&cfg.readTimeout, "read-timeout", envDur("WORMHOLE_READ_TIMEOUT", 30*time.Second),
		"timeout for reading a client request")
	flag.DurationVar(&cfg.idleTimeout, "idle-timeout", envDur("WORMHOLE_IDLE_TIMEOUT", 120*time.Second),
		"keep-alive timeout for idle client connections")
	flag.BoolVar(&cfg.verbose, "verbose", envBool("WORMHOLE_VERBOSE", false),
		"enable debug logging")
	flag.BoolVar(&cfg.showVersion, "version", false, "print version and exit")

	flag.Parse()
	return cfg
}

func envStr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envDur(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
