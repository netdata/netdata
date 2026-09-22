// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacyconfig"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/legacydelivery"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

func checkLegacy(ctx context.Context, paths []string) error {
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := openConfig(path)
		if err != nil {
			return fmt.Errorf("legacy configuration file %d: could not open file: %w", i+1, err)
		}
		_, err = legacyconfig.Parse(file)
		_ = file.Close()
		if err != nil {
			return fmt.Errorf("legacy configuration file %d: %w", i+1, err)
		}
	}
	return ctx.Err()
}

func executeLegacy(ctx context.Context, paths, methods, roles []string, stdin io.Reader, client *http.Client, runner *commandexec.Runner, report func(notifier.Result)) error {
	var programs []*legacyconfig.Program
	for i, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := openConfig(path)
		if err != nil {
			return fmt.Errorf("legacy configuration file %d: could not open file: %w", i+1, err)
		}
		program, err := legacyconfig.Parse(file)
		_ = file.Close()
		if err != nil {
			return fmt.Errorf("legacy configuration file %d: %w", i+1, err)
		}
		programs = append(programs, program)
	}
	notification, err := readNotification(stdin)
	if err != nil {
		return err
	}
	plan, names, err := legacydelivery.Prepare(ctx, programs, methods, roles, notification, client, runner)
	if err != nil {
		return err
	}
	return plan.Deliver(ctx, names, notification, report)
}
