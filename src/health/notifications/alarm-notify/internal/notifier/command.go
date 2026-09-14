// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

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
)

const usage = `Experimental Netdata notifier (not installed or used by the Agent).

Usage:
  alarm-notify validate --config FILE [--timeout 10s]
  alarm-notify send --config FILE --destination NAME [--timeout 10s] < event.json
  alarm-notify send --config FILE --role ROLE [--role ROLE ...] [--timeout 10s] < event.json

send reads one JSON event and delivers it to the selected destinations.
Use either one explicit destination or roles resolved through YAML routing.
validate checks configuration without resolving secrets or sending requests.
The positive timeout covers the whole invocation, including input reads.
Exit status: 0 on any successful delivery, no selected destinations, validation, or help;
1 on input/configuration errors, all deliveries failing, or command cancellation/timeout.
`

type commandUpdate struct {
	delivery *deliveryResult
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
	if len(args) == 0 || (args[0] != "send" && args[0] != "validate") {
		logger.Print("expected send or validate; use --help for usage")
		return 1
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	flags.SetOutput(io.Discard) // Flag errors can echo credential-bearing arguments.
	configPath := flags.String("config", "", "YAML configuration path")
	timeout := flags.Duration("timeout", 10*time.Second, "total invocation timeout")
	var destination string
	var roles []string
	if args[0] == "send" {
		flags.Func("destination", "named notification destination", func(value string) error {
			if strings.TrimSpace(value) == "" {
				return errors.New("destination must not be empty")
			}
			destination = value
			return nil
		})
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
	if flags.NArg() != 0 || *configPath == "" || *timeout <= 0 ||
		(args[0] == "send" && (destination == "") == (len(roles) == 0)) {
		logger.Print(
			"provide --config, a positive --timeout, and either --destination or --role for send; positional arguments are not accepted",
		)
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	// Keep output on this goroutine so cancellation cannot race with a caller's writer.
	updates := make(chan commandUpdate)
	publish := func(update commandUpdate) {
		select {
		case updates <- update:
		case <-ctx.Done():
		}
	}
	go func() {
		err := execute(
			ctx,
			*configPath,
			args[0] == "validate",
			destination,
			roles,
			stdin,
			*timeout,
			func(result deliveryResult) {
				publish(commandUpdate{delivery: &result})
			},
		)
		publish(commandUpdate{err: err})
	}()
	var err error
	succeeded, failed := 0, 0
receive:
	for {
		select {
		case update := <-updates:
			if update.delivery == nil {
				err = update.err
				break receive
			}
			if update.delivery.err != nil {
				failed++
				logger.Printf("destination %q failed: %s", update.delivery.destination, update.delivery.err)
			} else {
				succeeded++
				logger.Printf("destination %q sent", update.delivery.destination)
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
		logger.Printf("delivery summary: %d succeeded, %d failed", succeeded, failed)
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = errors.New("notification timed out")
		} else if errors.Is(err, context.Canceled) {
			err = errors.New("notification canceled")
		}
		logger.Print(err)
		return 1
	}
	if args[0] == "validate" {
		fmt.Fprintln(stdout, "configuration is valid")
	}
	return 0
}

func execute(
	ctx context.Context,
	configPath string,
	validateOnly bool,
	destination string,
	roles []string,
	stdin io.Reader,
	timeout time.Duration,
	report func(deliveryResult),
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(configPath)
	if err != nil {
		return errors.New("could not open configuration file")
	}
	cfg, err := readConfig(file)
	_ = file.Close()
	if err != nil {
		return err
	}
	if validateOnly {
		return nil
	}
	destinations, err := selectDestinations(cfg, destination, roles)
	if err != nil {
		return err
	}
	event, err := readEvent(stdin)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return dispatch(ctx, cfg, destinations, event, timeout, report)
}
