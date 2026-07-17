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
	"unicode"
)

const (
	functionResultPayloadLimit = 100 * 1024 * 1024
	functionResultLineLimit    = functionResultPayloadLimit + 64*1024
	functionResultGrace        = time.Second
	functionPermissions        = "0xFFFF"
	functionSource             = "method=api,role=admin"
	functionUID                = "functions-validation"
)

var (
	errFunctionNotPublished   = errors.New("Function was not published")
	errFunctionResultTooLarge = errors.New(
		"Function result exceeds aggregate payload limit",
	)
)

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
		cause := fmt.Errorf("read Agent protocol before publication: %w", err)
		return functionResult{}, stopAndJoin(
			stopProcess,
			waitCh,
			cfg.shutdownTimeout,
			cause,
		)
	case <-startupTimer.C:
		cause := fmt.Errorf(
			"%w after %s: %s",
			errFunctionNotPublished,
			cfg.startupTimeout,
			cfg.function,
		)
		return functionResult{}, stopAndJoin(
			stopProcess,
			waitCh,
			cfg.shutdownTimeout,
			cause,
		)
	case <-ctx.Done():
		return functionResult{}, stopAndJoin(
			stopProcess,
			waitCh,
			cfg.shutdownTimeout,
			ctx.Err(),
		)
	}

	request, err := encodeFunctionRequest(functionUID, cfg.function, cfg.args, cfg.functionTimeout)
	if err != nil {
		return functionResult{}, stopAndJoin(
			stopProcess,
			waitCh,
			cfg.shutdownTimeout,
			err,
		)
	}
	if _, err := io.WriteString(stdin, request); err != nil {
		cause := fmt.Errorf("write Function request: %w", err)
		return functionResult{}, stopAndJoin(
			stopProcess,
			waitCh,
			cfg.shutdownTimeout,
			cause,
		)
	}

	resultWait := protocolFunctionTimeout(cfg.functionTimeout) + functionResultGrace
	functionTimer := time.NewTimer(resultWait)
	defer functionTimer.Stop()

	var result functionResult
resultLoop:
	for {
		select {
		case result = <-events.result:
			break resultLoop
		case err := <-events.readErr:
			if buffered, ok := receiveBufferedResult(events.result); ok {
				result = buffered
				break resultLoop
			}
			cause := fmt.Errorf("read Agent protocol before result: %w", err)
			return functionResult{}, stopAndJoin(
				stopProcess,
				waitCh,
				cfg.shutdownTimeout,
				cause,
			)
		case <-functionTimer.C:
			if buffered, ok := receiveBufferedResult(events.result); ok {
				result = buffered
				break resultLoop
			}
			cause := fmt.Errorf("Function result timed out after %s", resultWait)
			return functionResult{}, stopAndJoin(
				stopProcess,
				waitCh,
				cfg.shutdownTimeout,
				cause,
			)
		case <-ctx.Done():
			if buffered, ok := receiveBufferedResult(events.result); ok {
				result = buffered
				break resultLoop
			}
			return functionResult{}, stopAndJoin(
				stopProcess,
				waitCh,
				cfg.shutdownTimeout,
				ctx.Err(),
			)
		}
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
		cause := fmt.Errorf(
			"Agent shutdown timed out after %s",
			cfg.shutdownTimeout,
		)
		return functionResult{}, stopAndJoin(
			stopProcess,
			waitCh,
			cfg.shutdownTimeout,
			cause,
		)
	case <-ctx.Done():
		return functionResult{}, stopAndJoin(
			stopProcess,
			waitCh,
			cfg.shutdownTimeout,
			ctx.Err(),
		)
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
	case !validFunctionToken(cfg.function):
		return errors.New("invalid --function token")
	case cfg.startupTimeout <= 0:
		return errors.New("startup timeout must be positive")
	case cfg.functionTimeout <= 0:
		return errors.New("Function timeout must be positive")
	case cfg.shutdownTimeout <= 0:
		return errors.New("shutdown timeout must be positive")
	default:
		for _, arg := range cfg.args {
			if !validFunctionToken(arg) {
				return errors.New("invalid --arg token")
			}
		}
		return nil
	}
}

func stopAndJoin(
	stop context.CancelFunc,
	waitCh <-chan error,
	timeout time.Duration,
	cause error,
) error {
	if err := stopAndWait(stop, waitCh, timeout); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func stopAndWait(
	stop context.CancelFunc,
	waitCh <-chan error,
	timeout time.Duration,
) error {
	stop()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waitCh:
		return nil
	case <-timer.C:
		return errors.New("Agent did not exit after forced termination")
	}
}

func encodeFunctionRequest(uid, function string, args []string, timeout time.Duration) (string, error) {
	if !validFunctionToken(uid) ||
		!validFunctionToken(function) ||
		timeout <= 0 {
		return "", errors.New("invalid Function request")
	}
	for _, arg := range args {
		if !validFunctionToken(arg) {
			return "", errors.New("invalid Function argument token")
		}
	}

	timeoutSeconds := int64(protocolFunctionTimeout(timeout) / time.Second)
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

func protocolFunctionTimeout(timeout time.Duration) time.Duration {
	seconds := timeout / time.Second
	if timeout%time.Second != 0 {
		seconds++
	}
	if seconds <= 0 {
		seconds = 1
	}
	return seconds * time.Second
}

func validFunctionToken(value string) bool {
	return value != "" &&
		strings.IndexFunc(value, func(r rune) bool {
			return unicode.IsSpace(r) || unicode.IsControl(r)
		}) == -1
}

func receiveBufferedResult(ch <-chan functionResult) (functionResult, bool) {
	select {
	case result := <-ch:
		return result, true
	default:
		return functionResult{}, false
	}
}

func readAgentProtocol(reader io.Reader, function, uid string, events protocolEvents) {
	readAgentProtocolWithLimit(
		reader,
		function,
		uid,
		functionResultPayloadLimit,
		events,
	)
}

func readAgentProtocolWithLimit(
	reader io.Reader,
	function string,
	uid string,
	payloadLimit int,
	events protocolEvents,
) {
	if payloadLimit < 0 {
		sendReadError(events.readErr, errFunctionResultTooLarge)
		return
	}

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
					result.payload = payload.Bytes()
					select {
					case events.result <- result:
					default:
					}
				}
				collecting = false
				skipping = false
				result = functionResult{}
				payload = bytes.Buffer{}
				continue
			}
			if collecting {
				additional := len(line)
				if payload.Len() > 0 {
					additional++
				}
				if additional > payloadLimit-payload.Len() {
					sendReadError(
						events.readErr,
						errFunctionResultTooLarge,
					)
					return
				}
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
	if collecting || skipping {
		sendReadError(events.readErr, io.ErrUnexpectedEOF)
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
