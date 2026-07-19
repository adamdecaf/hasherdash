package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/adamdecaf/hasherdash/internal/api"
	"github.com/adamdecaf/hasherdash/internal/config"
	"github.com/adamdecaf/hasherdash/internal/poller"
	"github.com/adamdecaf/hasherdash/internal/store"
)

func main() {
	configPath := flag.String("config", "", "path to config file (YAML or JSON); also CONFIG_FILE env")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	logger := log.New(os.Stdout, "", log.LstdFlags|log.Lmsgprefix)
	logger.Printf("hasherdash %s", cfg.Summary())

	if !cfg.HasDiscoveryTargets() {
		logger.Printf("warning: no ips/subnets/ranges configured — set them in hasherdash.yaml or MINER_SUBNET / MINER_IPS")
	}

	st := store.New(cfg.HistoryPoints, int(cfg.PollInterval.Seconds()))
	src := poller.NewSource(cfg)
	runner := poller.NewRunner(src, st, cfg.PollInterval, cfg.MinerTTL, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go runner.Run(ctx)

	srv := api.New(st, runner)
	httpServer := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Printf("listening on %s", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("http: %v", err)
		}
	}()

	<-ctx.Done()
	logger.Printf("shutting down…")
	shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	_ = httpServer.Shutdown(shutdownCtx)
}
