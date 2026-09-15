// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return notifier.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
}
