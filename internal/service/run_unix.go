//go:build !windows

package service

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func runService(cfg Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, cfg)
}
