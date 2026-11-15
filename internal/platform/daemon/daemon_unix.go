//go:build !windows

package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// run cancels ctx on SIGINT/SIGTERM, which is how systemd and launchd stop a
// service: `systemctl stop` sends SIGTERM and expects the process to exit.
func run(_ string, serve func(ctx context.Context) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serve(ctx)
}
