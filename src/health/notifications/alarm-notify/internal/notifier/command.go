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
	"time"
)

const usage = `Experimental Netdata notifier (not installed or used by the Agent).

Usage:
  alarm-notify validate --config FILE [--timeout 10s]
  alarm-notify send --config FILE --destination NAME [--timeout 10s] < event.json

send reads one JSON event and posts it to one configured webhook.
validate checks configuration without resolving secrets or sending requests.
The positive timeout covers the whole invocation, including input reads.
Exit status: 0 on success/help; 1 on input, configuration, or delivery failure.
`

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
	if args[0] == "send" {
		flags.StringVar(&destination, "destination", "", "named webhook destination")
	}
	if err := flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stdout, usage)
			return 0
		}
		logger.Print("invalid command options; use --help for usage")
		return 1
	}
	if flags.NArg() != 0 || *configPath == "" || *timeout <= 0 || (args[0] == "send" && destination == "") {
		logger.Print(
			"provide --config, a positive --timeout, and --destination for send; positional arguments are not accepted",
		)
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	// Keep output on this goroutine so cancellation cannot race with a caller's writer.
	result := make(chan error, 1)
	go func() {
		result <- execute(ctx, *configPath, destination, stdin, *timeout)
	}()
	var err error
	select {
	case err = <-result:
		if ctx.Err() != nil {
			err = ctx.Err()
		}
	case <-ctx.Done():
		err = ctx.Err()
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
	} else {
		logger.Print("notification sent")
	}
	return 0
}

func execute(ctx context.Context, configPath, destination string, stdin io.Reader, timeout time.Duration) error {
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
	if destination == "" {
		return nil
	}
	dst, ok := cfg.Destinations[destination]
	if !ok {
		return errors.New("selected destination is not configured")
	}
	event, err := readEvent(stdin)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return sendWebhook(ctx, dst, event, timeout)
}
