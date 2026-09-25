// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers"
)

const usage = `Experimental Netdata notifier (not installed or used by the Agent).

Usage:
  alarm-notify send-legacy --config FILE [--config FILE ...] --role ROLE [--role ROLE ...] [--method METHOD ...] [--timeout 10s] < event.json
  alarm-notify check-legacy --config FILE [--config FILE ...] [--timeout 10s]
  alarm-notify validate --config FILE [--timeout 10s]
  alarm-notify send --config FILE --destination NAME [--timeout 10s] < event.json
  alarm-notify send --config FILE --role ROLE [--role ROLE ...] [--timeout 10s] < event.json

send-legacy evaluates supported shell-format settings and uses the existing Go senders.
It considers all configured methods unless --method selects a supported subset.
Eligible unsupported methods/settings reject the invocation before any delivery.
Config files are applied in order; custom Bash functions are never executed.
send reads one JSON event and delivers it to the selected destinations.
Use either one explicit destination or roles resolved through YAML routing.
Destination critical/nowarn/noclear policies filter delivery. Missing required
critical_seen_since_clear history rejects the invocation before any delivery.
validate checks YAML configuration without resolving secrets or sending requests.
check-legacy checks the supported shell configuration syntax without executing it;
provider settings, variable values and custom-function behavior are not validated.
The positive timeout covers the whole invocation, including input reads.
Exit status: 0 on any successful delivery, no eligible destinations, successful checks, or help;
1 on input/configuration errors, all attempted deliveries failing, or cancellation/timeout.
`

type commandUpdate struct {
	delivery *notifier.Result
	err      error
}

// Run implements the command boundary. On cancellation the process may exit while
// an input read is blocked; callers that reuse Run must close their input afterward.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	logger := log.New(stderr, "alarm-notify: ", 0)
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h" || args[0] == "help") {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if len(args) == 0 || (args[0] != "send" && args[0] != "send-legacy" && args[0] != "validate" && args[0] != "check-legacy") {
		logger.Print("expected send, send-legacy, validate or check-legacy; use --help for usage")
		return 1
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard) // Flag errors can echo credential-bearing arguments.
	var configPaths []string
	flags.Func("config", "configuration path (repeat in load order for legacy commands)", func(value string) error {
		configPaths = append(configPaths, value)
		return nil
	})
	timeout := flags.Duration("timeout", 10*time.Second, "total invocation timeout")
	var destination string
	var roles, methods []string
	if args[0] == "send" {
		flags.Func("destination", "named notification destination", func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("destination must not be empty")
			}
			destination = value
			return nil
		})
	}
	if args[0] == "send-legacy" {
		flags.Func("method", "legacy method to include (repeat to select multiple)", func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("method must not be empty")
			}
			methods = append(methods, value)
			return nil
		})
	}
	if args[0] == "send" || args[0] == "send-legacy" {
		flags.Func("role", "role to notify (repeat for multiple roles)", func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("role must not be empty")
			}
			roles = append(roles, value)
			return nil
		})
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		logger.Print("invalid command options; use --help for usage")
		return 1
	}
	if flags.NArg() != 0 || len(configPaths) == 0 || configPaths[len(configPaths)-1] == "" || *timeout <= 0 ||
		(args[0] == "send" && (destination == "") == (len(roles) == 0)) ||
		(args[0] == "send-legacy" && len(roles) == 0) {
		if args[0] == "send" {
			logger.Print("provide --config, a positive --timeout, and either --destination or --role for send; positional arguments are not accepted")
		} else if args[0] == "send-legacy" {
			logger.Print("provide --config, --role and a positive --timeout for send-legacy; positional arguments are not accepted")
		} else {
			logger.Print("provide --config and a positive --timeout; positional arguments are not accepted")
		}
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	processes := &commandexec.Runner{}
	client := httpclient.New(*timeout)
	defer func() {
		cancel()
		processes.CloseAndWait()
		client.CloseIdleConnections()
	}()
	// Keep output on this goroutine so cancellation cannot race with a caller's writer.
	updates := make(chan commandUpdate)
	publish := func(update commandUpdate) {
		select {
		case updates <- update:
		case <-ctx.Done():
		}
	}
	go func() {
		if args[0] == "check-legacy" {
			publish(commandUpdate{err: checkLegacy(ctx, configPaths)})
			return
		}
		report := func(result notifier.Result) { publish(commandUpdate{delivery: &result}) }
		if args[0] == "send-legacy" {
			err := executeLegacy(ctx, configPaths, methods, roles, stdin, client, processes, report)
			publish(commandUpdate{err: err})
			return
		}
		err := execute(
			ctx,
			providers.Builtin(client, processes),
			configPaths[len(configPaths)-1],
			args[0] == "validate",
			destination,
			roles,
			stdin,
			report,
		)
		publish(commandUpdate{err: err})
	}()
	var err error
	succeeded, failed, skipped := 0, 0, 0
receive:
	for {
		select {
		case update := <-updates:
			if update.delivery == nil {
				err = update.err
				break receive
			}
			if update.delivery.SkipReason != "" {
				skipped++
				logger.Printf("destination %q skipped: %s", update.delivery.Destination, update.delivery.SkipReason)
			} else if update.delivery.Err != nil {
				failed++
				logger.Printf("destination %q failed: %s", update.delivery.Destination, update.delivery.Err)
			} else {
				succeeded++
				logger.Printf("destination %q sent", update.delivery.Destination)
			}
		case <-ctx.Done():
			err = ctx.Err()
			break receive
		}
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if args[0] == "send" {
		logger.Printf("delivery summary: %d succeeded, %d failed, %d skipped", succeeded, failed, skipped)
	} else if args[0] == "send-legacy" {
		logger.Printf("delivery summary: %d succeeded, %d failed", succeeded, failed)
	}
	if err != nil {
		subject := "notification"
		switch args[0] {
		case "check-legacy":
			subject = "legacy configuration check"
		case "validate":
			subject = "configuration validation"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("%s timed out", subject)
		} else if errors.Is(err, context.Canceled) {
			err = fmt.Errorf("%s canceled", subject)
		}
		logger.Print(err)
		return 1
	}
	if args[0] == "validate" {
		fmt.Fprintln(stdout, "configuration is valid")
	} else if args[0] == "check-legacy" {
		fmt.Fprintln(stdout, "legacy configuration syntax is supported; delivery is not validated")
	}
	return 0
}

func execute(
	ctx context.Context,
	registry config.Registry,
	configPath string,
	validateOnly bool,
	destination string,
	roles []string,
	stdin io.Reader,
	report func(notifier.Result),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := openConfig(configPath)
	if err != nil {
		return fmt.Errorf("could not open configuration file: %w", err)
	}
	cfg, err := config.Read(file, registry)
	_ = file.Close()
	if err != nil {
		return err
	}
	if validateOnly {
		return nil
	}
	destinations, err := cfg.Select(destination, roles)
	if err != nil {
		return err
	}
	notification, err := readNotification(stdin)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return cfg.Deliver(ctx, destinations, notification, report)
}

func openConfig(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err == nil {
		return file, nil
	}
	// Omit the configured path while preserving the filesystem cause and errors.Is.
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return nil, pathErr.Err
	}
	return nil, errors.New("unknown filesystem error")
}
