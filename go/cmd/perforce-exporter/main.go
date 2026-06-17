// Boring on purpose: load config, register the collector, serve /metrics,
// wait for a signal.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ssephi/perforce-prom-exporter/internal/collector"
	"github.com/ssephi/perforce-prom-exporter/internal/config"
)

// Populated by `go build -ldflags="-X main.version=…"` at release time
// (GoReleaser fills these in). Default to "dev" for plain `go build`.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if code := run(); code != 0 {
		os.Exit(code)
	}
}

func run() int {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// `--version` is the only flag we handle by hand. Anything else falls
	// through to the env-driven config path so behaviour matches the
	// Python exporter (no flag surface).
	for _, a := range os.Args[1:] {
		if a == "--version" || a == "-v" {
			fmt.Printf("perforce-exporter %s (commit %s, built %s)\n", version, commit, date)
			return 0
		}
	}
	log.Printf("perforce-exporter version=%s commit=%s built=%s", version, commit, date)

	cfg, err := config.Load(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		return 2
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector.New(cfg))

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintln(w, `<html><body><h1>perforce-prom-exporter</h1><p><a href="/metrics">/metrics</a></p></body></html>`)
	})

	addr := fmt.Sprintf(":%d", cfg.ListenPort)
	srv := &http.Server{Addr: addr, Handler: mux}

	targetSummary := make([]string, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		targetSummary = append(targetSummary, t.Name+"="+t.Port)
	}
	log.Printf("listening on %s targets=%s", addr, strings.Join(targetSummary, ","))

	serveErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	// signal handling: portable subset that works on both unix and windows.
	// (syscall.SIGTERM is handled via signal.Notify on platforms that
	// support it; os.Interrupt covers Ctrl-C and Windows.)
	ctx, stop := signal.NotifyContext(context.Background(), interruptSignals()...)
	defer stop()

	select {
	case err := <-serveErr:
		log.Printf("http server exited: %v", err)
		return 1
	case <-ctx.Done():
		log.Printf("shutdown requested")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	return 0
}
