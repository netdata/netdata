// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	functionResultLineLimit = 101 * 1024 * 1024
	functionPermissions     = "0xFFFF"
	functionSource          = "method=api,role=admin"
	functionUID             = "functions-validation"
)

var errFunctionNotPublished = errors.New("Function was not published")

type callConfig struct {
	pluginPath      string
	configDir       string
	module          string
	function        string
	args            []string
	startupTimeout  time.Duration
	functionTimeout time.Duration
	shutdownTimeout time.Duration
	stderr          io.Writer
}

type functionResult struct {
	status      int
	contentType string
	expiry      int64
	payload     []byte
}

type protocolEvents struct {
	published chan struct{}
	result    chan functionResult
	readErr   chan error
}

func callAgent(ctx context.Context, cfg callConfig) (functionResult, error) {
	if err := cfg.validate(); err != nil {
		return functionResult{}, err
	}

	processCtx, stopProcess := context.WithCancel(ctx)
	defer stopProcess()

	cmd := exec.CommandContext(processCtx, cfg.pluginPath,
		"--config-dir", cfg.configDir,
		"--modules", cfg.module,
	)
	cmd.Stderr = cfg.stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return functionResult{}, fmt.Errorf("open Agent stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return functionResult{}, fmt.Errorf("open Agent stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return functionResult{}, fmt.Errorf("start Agent: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	events := protocolEvents{
		published: make(chan struct{}, 1),
		result:    make(chan functionResult, 1),
		readErr:   make(chan error, 1),
	}
	go readAgentProtocol(stdout, cfg.function, functionUID, events)

	startupTimer := time.NewTimer(cfg.startupTimeout)
	defer startupTimer.Stop()

	select {
	case <-events.published:
	case err := <-events.readErr:
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, fmt.Errorf("read Agent protocol before publication: %w", err)
	case err := <-waitCh:
		return functionResult{}, fmt.Errorf("Agent exited before Function publication: %w", err)
	case <-startupTimer.C:
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, fmt.Errorf("%w after %s: %s", errFunctionNotPublished, cfg.startupTimeout, cfg.function)
	case <-ctx.Done():
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, ctx.Err()
	}

	request, err := encodeFunctionRequest(functionUID, cfg.function, cfg.args, cfg.functionTimeout)
	if err != nil {
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, err
	}
	if _, err := io.WriteString(stdin, request); err != nil {
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, fmt.Errorf("write Function request: %w", err)
	}

	functionTimer := time.NewTimer(cfg.functionTimeout)
	defer functionTimer.Stop()

	var result functionResult
	select {
	case result = <-events.result:
	case err := <-events.readErr:
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, fmt.Errorf("read Agent protocol before result: %w", err)
	case err := <-waitCh:
		return functionResult{}, fmt.Errorf("Agent exited before Function result: %w", err)
	case <-functionTimer.C:
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, fmt.Errorf("Function result timed out after %s", cfg.functionTimeout)
	case <-ctx.Done():
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, ctx.Err()
	}

	_, _ = io.WriteString(stdin, "QUIT\n")
	_ = stdin.Close()

	shutdownTimer := time.NewTimer(cfg.shutdownTimeout)
	defer shutdownTimer.Stop()
	select {
	case err := <-waitCh:
		if err != nil {
			return functionResult{}, fmt.Errorf("Agent shutdown: %w", err)
		}
	case <-shutdownTimer.C:
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, fmt.Errorf("Agent shutdown timed out after %s", cfg.shutdownTimeout)
	case <-ctx.Done():
		stopAndWait(stopProcess, waitCh, cfg.shutdownTimeout)
		return functionResult{}, ctx.Err()
	}

	return result, nil
}

func (cfg callConfig) validate() error {
	switch {
	case cfg.pluginPath == "":
		return errors.New("missing --plugin")
	case cfg.configDir == "":
		return errors.New("missing --config-dir")
	case cfg.module == "":
		return errors.New("missing --module")
	case cfg.function == "":
		return errors.New("missing --function")
	case cfg.startupTimeout <= 0:
		return errors.New("startup timeout must be positive")
	case cfg.functionTimeout <= 0:
		return errors.New("Function timeout must be positive")
	case cfg.shutdownTimeout <= 0:
		return errors.New("shutdown timeout must be positive")
	default:
		return nil
	}
}

func stopAndWait(stop context.CancelFunc, waitCh <-chan error, timeout time.Duration) {
	stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waitCh:
	case <-timer.C:
	}
}

func encodeFunctionRequest(uid, function string, args []string, timeout time.Duration) (string, error) {
	if uid == "" || function == "" || timeout <= 0 {
		return "", errors.New("invalid Function request")
	}

	timeoutSeconds := int64((timeout + time.Second - 1) / time.Second)
	nameAndArgs := strings.Join(append([]string{function}, args...), " ")

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.Comma = ' '
	if err := writer.Write([]string{
		"FUNCTION",
		uid,
		strconv.FormatInt(timeoutSeconds, 10),
		nameAndArgs,
		functionPermissions,
		functionSource,
	}); err != nil {
		return "", fmt.Errorf("encode Function request: %w", err)
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", fmt.Errorf("encode Function request: %w", err)
	}
	return buf.String(), nil
}

func readAgentProtocol(reader io.Reader, function, uid string, events protocolEvents) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), functionResultLineLimit)

	var (
		collecting bool
		skipping   bool
		result     functionResult
		payload    bytes.Buffer
	)

	for scanner.Scan() {
		line := scanner.Text()

		if collecting || skipping {
			if line == "FUNCTION_RESULT_END" {
				if collecting {
					result.payload = append([]byte(nil), payload.Bytes()...)
					select {
					case events.result <- result:
					default:
					}
				}
				collecting = false
				skipping = false
				result = functionResult{}
				payload.Reset()
				continue
			}
			if collecting {
				if payload.Len() > 0 {
					payload.WriteByte('\n')
				}
				payload.WriteString(line)
			}
			continue
		}

		if publishedFunction(line) == function {
			select {
			case events.published <- struct{}{}:
			default:
			}
			continue
		}

		header, ok, err := parseFunctionResultHeader(line)
		if err != nil {
			sendReadError(events.readErr, err)
			return
		}
		if !ok {
			continue
		}
		if header.uid != uid {
			skipping = true
			continue
		}
		collecting = true
		result = header.result
	}

	if err := scanner.Err(); err != nil {
		sendReadError(events.readErr, err)
		return
	}
	sendReadError(events.readErr, io.EOF)
}

func publishedFunction(line string) string {
	reader := csv.NewReader(strings.NewReader(line))
	reader.Comma = ' '
	fields, err := reader.Read()
	if err != nil || len(fields) < 3 || fields[0] != "FUNCTION" || fields[1] != "GLOBAL" {
		return ""
	}
	return fields[2]
}

type resultHeader struct {
	uid    string
	result functionResult
}

func parseFunctionResultHeader(line string) (resultHeader, bool, error) {
	if !strings.HasPrefix(line, "FUNCTION_RESULT_BEGIN ") {
		return resultHeader{}, false, nil
	}
	fields := strings.Fields(line)
	if len(fields) != 5 {
		return resultHeader{}, false, errors.New("malformed FUNCTION_RESULT_BEGIN")
	}
	status, err := strconv.Atoi(fields[2])
	if err != nil {
		return resultHeader{}, false, fmt.Errorf("parse Function result status: %w", err)
	}
	if status < 100 || status > 599 {
		return resultHeader{}, false, errors.New("Function result status is outside 100..599")
	}
	expiry, err := strconv.ParseInt(fields[4], 10, 64)
	if err != nil {
		return resultHeader{}, false, fmt.Errorf("parse Function result expiry: %w", err)
	}
	return resultHeader{
		uid: fields[1],
		result: functionResult{
			status:      status,
			contentType: fields[3],
			expiry:      expiry,
		},
	}, true, nil
}

func sendReadError(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}
