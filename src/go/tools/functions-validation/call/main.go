// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

type stringList []string

func (values *stringList) String() string {
	return fmt.Sprint([]string(*values))
}

func (values *stringList) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	var (
		cfg  callConfig
		args stringList
	)

	flag.StringVar(&cfg.pluginPath, "plugin", "", "path to go.d.plugin")
	flag.StringVar(&cfg.configDir, "config-dir", "", "plugin configuration directory")
	flag.StringVar(&cfg.module, "module", "", "collector module to run")
	flag.StringVar(&cfg.function, "function", "", "published Function name")
	flag.Var(&args, "arg", "Function argument token (repeatable)")
	flag.DurationVar(&cfg.startupTimeout, "startup-timeout", 10*time.Second, "maximum wait for Function publication")
	flag.DurationVar(&cfg.functionTimeout, "timeout", time.Minute, "Function request timeout")
	flag.DurationVar(&cfg.shutdownTimeout, "shutdown-timeout", 15*time.Second, "maximum wait for Agent shutdown")
	flag.Parse()
	cfg.args = args
	cfg.stderr = os.Stderr

	overall := cfg.startupTimeout +
		protocolFunctionTimeout(cfg.functionTimeout) +
		functionResultGrace +
		cfg.shutdownTimeout +
		time.Second
	ctx, cancel := context.WithTimeout(context.Background(), overall)
	defer cancel()

	result, err := callAgent(ctx, cfg)
	if err != nil {
		if errors.Is(err, errFunctionNotPublished) {
			writeControlError(503, fmt.Sprintf("no jobs started for module '%s'", cfg.module))
		}
		_, _ = fmt.Fprintf(os.Stderr, "Function validation failed: %v\n", err)
		os.Exit(1)
	}

	_, _ = os.Stdout.Write(result.payload)
	_, _ = os.Stdout.Write([]byte("\n"))
	if result.status >= 400 {
		os.Exit(1)
	}
}

func writeControlError(status int, message string) {
	payload, err := json.Marshal(map[string]any{
		"status":       status,
		"errorMessage": message,
	})
	if err != nil {
		return
	}
	_, _ = os.Stdout.Write(payload)
	_, _ = os.Stdout.Write([]byte("\n"))
}
