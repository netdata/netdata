// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_jmx

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

// To build the JAR file, run: ./build_java_helper.sh
// This will compile websphere_jmx_helper.java and create the JAR file.
// The JAR must be built before compiling the Go code.
//
//go:embed "websphere_jmx_helper.jar"
var jmxHelperJar []byte

const jmxHelperTempFile = "netdata_websphere_jmx_helper.jar"

type jmxCommand struct {
	Command         string          `json:"command"`
	ProtocolVersion string          `json:"protocol_version,omitempty"`
	JMXURL          string          `json:"jmx_url,omitempty"`
	JMXUsername     string          `json:"jmx_username,omitempty"`
	JMXPassword     string          `json:"jmx_password,omitempty"`
	JMXClasspath    string          `json:"jmx_classpath,omitempty"`
	Target          string          `json:"target,omitempty"`
	MaxItems        int             `json:"max_items,omitempty"`
	CollectOptions  map[string]bool `json:"collect_options,omitempty"`
}

type jmxResponse struct {
	Status      string                 `json:"status"`
	Message     string                 `json:"message,omitempty"`
	Details     string                 `json:"details,omitempty"`
	Recoverable bool                   `json:"recoverable,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

type jmxHelper struct {
	config Config
	logger logger.Logger

	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	stderr  io.ReadCloser
	scanner *bufio.Scanner
	mu      sync.Mutex

	jarPath string
	running bool
}

func newJMXHelper(config Config, logger logger.Logger) (*jmxHelper, error) {
	return &jmxHelper{
		config: config,
		logger: logger,
	}, nil
}

func (h *jmxHelper) start(ctx context.Context) error {
	// Write JAR file to temp directory
	jarPath, err := h.writeJARFile()
	if err != nil {
		return fmt.Errorf("failed to write JAR file: %w", err)
	}
	h.jarPath = jarPath

	// Prepare Java command
	javaPath := h.config.JavaExecPath
	if javaPath == "" {
		javaPath = "java"
	}

	args := []string{"-jar", jarPath}
	h.cmd = exec.CommandContext(ctx, javaPath, args...)

	// Set up pipes
	h.stdin, err = h.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	h.stdout, err = h.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	h.stderr, err = h.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}

	h.scanner = bufio.NewScanner(h.stdout)

	// Start the process
	if err := h.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start JMX helper: %w", err)
	}

	h.running = true

	// Start stderr logger
	go h.logStderr()

	// Send initialization command
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	initCmd := jmxCommand{
		Command:         "INIT",
		ProtocolVersion: "1.0",
		JMXURL:          h.config.JMXURL,
		JMXUsername:     h.config.JMXUsername,
		JMXPassword:     h.config.JMXPassword,
		JMXClasspath:    h.config.JMXClasspath,
	}

	resp, err := h.sendCommand(initCtx, initCmd)
	if err != nil {
		h.shutdown()
		return fmt.Errorf("failed to initialize JMX connection: %w", err)
	}

	if resp.Status != "OK" {
		h.shutdown()
		return fmt.Errorf("JMX initialization failed: %s", resp.Message)
	}

	h.logger.Info("JMX helper initialized successfully")
	return nil
}

func (h *jmxHelper) sendCommand(ctx context.Context, cmd jmxCommand) (*jmxResponse, error) {
	if !h.running {
		return nil, fmt.Errorf("JMX helper is not running")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Encode and send command
	cmdData, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal command: %w", err)
	}

	if _, err := fmt.Fprintln(h.stdin, string(cmdData)); err != nil {
		return nil, fmt.Errorf("failed to send command: %w", err)
	}

	// Read response with timeout
	respChan := make(chan *jmxResponse, 1)
	errChan := make(chan error, 1)

	go func() {
		if !h.scanner.Scan() {
			if err := h.scanner.Err(); err != nil {
				errChan <- fmt.Errorf("failed to read response: %w", err)
			} else {
				errChan <- fmt.Errorf("unexpected EOF from JMX helper")
			}
			return
		}

		var resp jmxResponse
		if err := json.Unmarshal(h.scanner.Bytes(), &resp); err != nil {
			errChan <- fmt.Errorf("failed to unmarshal response: %w", err)
			return
		}

		respChan <- &resp
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errChan:
		return nil, err
	case resp := <-respChan:
		return resp, nil
	}
}

func (h *jmxHelper) shutdown() {
	if !h.running {
		return
	}

	h.running = false

	// Send shutdown command
	shutdownCmd := jmxCommand{
		Command: "SHUTDOWN",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := h.sendCommand(ctx, shutdownCmd); err != nil {
		h.logger.Debugf("failed to send shutdown command: %v", err)
	}

	// Give the process time to shut down gracefully
	time.Sleep(time.Duration(h.config.ShutdownDelay))

	// Force kill if still running
	if h.cmd.Process != nil {
		if err := h.cmd.Process.Kill(); err != nil {
			h.logger.Debugf("failed to kill JMX helper process: %v", err)
		}
	}

	// Wait for process to exit
	if err := h.cmd.Wait(); err != nil {
		h.logger.Debugf("JMX helper process exited with error: %v", err)
	}

	// Clean up JAR file
	if h.jarPath != "" {
		if err := os.Remove(h.jarPath); err != nil {
			h.logger.Debugf("failed to remove JAR file: %v", err)
		}
	}

	h.logger.Info("JMX helper shut down")
}

func (h *jmxHelper) writeJARFile() (string, error) {
	jarPath := filepath.Join(os.TempDir(), jmxHelperTempFile)

	if err := os.WriteFile(jarPath, jmxHelperJar, 0644); err != nil {
		return "", fmt.Errorf("failed to write JAR file: %w", err)
	}

	return jarPath, nil
}

func (h *jmxHelper) logStderr() {
	scanner := bufio.NewScanner(h.stderr)
	for scanner.Scan() {
		h.logger.Errorf("[websphere-jmx-helper] %s", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		h.logger.Errorf("error reading stderr: %v", err)
	}
}
