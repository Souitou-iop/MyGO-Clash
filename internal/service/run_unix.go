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
	fixUnit(cfg)
	return Run(ctx, cfg)
}
