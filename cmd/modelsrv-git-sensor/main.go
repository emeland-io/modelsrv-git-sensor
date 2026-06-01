package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"emeland.io/modelsrv-git-sensor/internal/config"
	"emeland.io/modelsrv-git-sensor/internal/reconcile"
)

func main() {
	var (
		configPath   = flag.String("config", envOrDefault("SENSOR_CONFIG", "config/sensor.yaml"), "Path to YAML config")
		listenAddr   = flag.String("listen", envOrDefault("SENSOR_LISTEN_ADDR", "localhost:24100"), "HTTP listen address for this sensor's modelsrv API")
		pollInterval = flag.Duration("poll-interval", envDurationOrDefault("SENSOR_POLL_INTERVAL", 10*time.Second), "Reconcile polling interval")
	)
	flag.Parse()

	// Development logger attaches stack traces to Warn+ by default, which looks
	// like a panic for benign warnings (e.g. missing scan path). Only attach
	// stacks at Error and above.
	log := zap.Must(zap.NewDevelopmentConfig().Build(zap.AddStacktrace(zap.ErrorLevel)))
	slog := log.Sugar()
	defer func() { _ = log.Sync() }()

	cfgFile, err := config.Load(*configPath)
	if err != nil {
		slog.Fatalw("failed to load config", "path", *configPath, "error", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := reconcile.Config{
		ListenAddr:  *listenAddr,
		Subscribers: cfgFile.Subscribers,
		Repos:        make([]reconcile.RepoConfig, 0, len(cfgFile.Repos)),
		PollInterval: *pollInterval,
		WatchFS:      cfgFile.Watch,
	}
	for _, r := range cfgFile.Repos {
		cfg.Repos = append(cfg.Repos, reconcile.RepoConfig{
			Type:        reconcile.ParseRepoType(r.Type),
			DeployKey:   r.DeployKey,
			Repo:        r.Repo,
			Branch:      r.Branch,
			CheckoutDir: r.CheckoutDir,
			Paths:       r.Paths,
		})
	}

	if err := reconcile.Run(ctx, cfg, slog); err != nil {
		slog.Fatalw("sensor exited with error", "error", err)
	}
}

func envOrDefault(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
