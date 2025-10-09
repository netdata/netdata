// Package jmxbridge provides process management for JSON-based Java helper bridges.
// SPDX-License-Identifier: GPL-3.0-or-later

package jmxbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// Logger is the minimal logging contract required by the bridge.
type Logger interface {
	Debugf(format string, args ...any)
	Infof(format string, args ...any)
	Warningf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Config contains options for launching the helper process.
type Config struct {
	JavaExecPath string
	JarPath      string
	JarData      []byte
	JarFileName  string
	WorkingDir   string
}

// Command represents a JSON command sent to the helper.
type Command map[string]interface{}

// Response represents a generic helper response.
type Response struct {
	Status      string                 `json:"status"`
	Message     string                 `json:"message,omitempty"`
	Details     string                 `json:"details,omitempty"`
	Recoverable bool                   `json:"recoverable,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// Option configures a Client.
type Option func(*Client)

// ProcessFactory builds a process abstraction. It may be overridden for tests.
type ProcessFactory func(ctx context.Context, javaPath string, jarPath string) (process, error)

type process interface {
	Start() error
	Stdin() io.WriteCloser
	Stdout() io.ReadCloser
	Stderr() io.ReadCloser
	Wait() error
}

// Client manages the lifecycle of the helper process.
type Client struct {
	cfg    Config
	logger Logger

	processFactory ProcessFactory

	mu       sync.Mutex
	proc     process
	cancel   context.CancelFunc
	stdin    io.WriteCloser
	scanner  *bufio.Scanner
	stderr   io.ReadCloser
	stderrWG sync.WaitGroup
	running  bool

	jarPath   string
	removeJar bool
}

// NewClient creates a Client with optional customisations.
func NewClient(cfg Config, logger Logger, opts ...Option) (*Client, error) {
	if logger == nil {
		return nil, errors.New("jmxbridge: logger is required")
	}

	if cfg.JarPath == "" && len(cfg.JarData) == 0 {
		return nil, errors.New("jmxbridge: either JarPath or JarData must be provided")
	}

	if cfg.JarFileName == "" {
		cfg.JarFileName = "netdata_jmx_helper.jar"
	}

	client := &Client{
		cfg:            cfg,
		logger:         logger,
		processFactory: defaultProcessFactory,
	}

	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

// WithProcessFactory overrides the process factory (useful for tests).
func WithProcessFactory(factory ProcessFactory) Option {
	return func(c *Client) {
		c.processFactory = factory
	}
}

// Start launches the helper and sends the initial command.
func (c *Client) Start(ctx context.Context, initCmd Command) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	jarPath, removeJar, err := c.prepareJar()
	if err != nil {
		return err
	}

	procCtx, cancel := context.WithCancel(context.Background())
	proc, err := c.processFactory(procCtx, c.cfg.JavaExecPath, jarPath)
	if err != nil {
		cancel()
		if removeJar {
			_ = os.Remove(jarPath)
		}
		return err
	}

	stdin := proc.Stdin()
	stdout := proc.Stdout()
	stderr := proc.Stderr()

	if err := proc.Start(); err != nil {
		cancel()
		if removeJar {
			_ = os.Remove(jarPath)
		}
		return fmt.Errorf("jmxbridge: failed to start helper: %w", err)
	}

	c.stderrWG.Add(1)
	go c.consumeStderr(stderr)

	c.proc = proc
	c.cancel = cancel
	c.stdin = stdin
	c.scanner = bufio.NewScanner(stdout)
	c.stderr = stderr
	c.running = true
	c.jarPath = jarPath
	c.removeJar = removeJar

	if initCmd != nil {
		c.mu.Unlock()
		_, err := c.Send(ctx, initCmd)
		c.mu.Lock()
		if err != nil {
			c.internalShutdownLocked()
			return fmt.Errorf("jmxbridge: helper init failed: %w", err)
		}
	}

	c.logger.Infof("jmxbridge: helper started")
	return nil
}

// Send transmits a command and waits for the corresponding response.
func (c *Client) Send(ctx context.Context, cmd Command) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil, errors.New("jmxbridge: helper not running")
	}

	payload, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("jmxbridge: failed to marshal command: %w", err)
	}

	if _, err := c.stdin.Write(append(payload, '\n')); err != nil {
		return nil, fmt.Errorf("jmxbridge: failed to send command: %w", err)
	}

	scanner := c.scanner
	if scanner == nil {
		return nil, errors.New("jmxbridge: response scanner not initialised")
	}

	type result struct {
		line string
		err  error
	}

	resCh := make(chan result, 1)
	go func(s *bufio.Scanner) {
		if s.Scan() {
			resCh <- result{line: s.Text()}
		} else {
			resCh <- result{err: s.Err()}
		}
	}(scanner)

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("jmxbridge: waiting for response cancelled: %w", ctx.Err())
	case res := <-resCh:
		if res.err != nil {
			return nil, fmt.Errorf("jmxbridge: failed to read response: %w", res.err)
		}
		var resp Response
		if err := json.Unmarshal([]byte(res.line), &resp); err != nil {
			return nil, fmt.Errorf("jmxbridge: invalid response: %w", err)
		}
		status := strings.ToUpper(resp.Status)
		if status != "OK" && status != "SUCCESS" {
			return &resp, fmt.Errorf("jmxbridge: helper returned status %s: %s", resp.Status, resp.Message)
		}
		return &resp, nil
	}
}

// Shutdown terminates the helper process and cleans resources.
func (c *Client) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.internalShutdownLocked()
}

func (c *Client) internalShutdownLocked() {
	if !c.running {
		return
	}

	if c.cancel != nil {
		c.cancel()
	}

	if c.stdin != nil {
		_ = c.stdin.Close()
	}

	if c.proc != nil {
		_ = c.proc.Wait()
	}

	if c.stderr != nil {
		c.stderr.Close()
	}
	c.stderrWG.Wait()

	if c.removeJar {
		_ = os.Remove(c.jarPath)
	}

	c.proc = nil
	c.stdin = nil
	c.scanner = nil
	c.stderr = nil
	c.cancel = nil
	c.running = false
	c.logger.Infof("jmxbridge: helper stopped")
}

func (c *Client) prepareJar() (string, bool, error) {
	if c.cfg.JarPath != "" {
		return c.cfg.JarPath, false, nil
	}

	dir := c.cfg.WorkingDir
	if dir == "" {
		dir = os.TempDir()
	}

	f, err := os.CreateTemp(dir, c.cfg.JarFileName)
	if err != nil {
		return "", false, fmt.Errorf("jmxbridge: failed to create temp jar: %w", err)
	}
	if _, err := f.Write(c.cfg.JarData); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", false, fmt.Errorf("jmxbridge: failed to write jar: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", false, fmt.Errorf("jmxbridge: failed to close jar: %w", err)
	}

	return f.Name(), true, nil
}

func (c *Client) consumeStderr(r io.Reader) {
	defer c.stderrWG.Done()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		c.logger.Debugf("jmxbridge stderr: %s", scanner.Text())
	}
}

func defaultProcessFactory(ctx context.Context, javaPath, jarPath string) (process, error) {
	if javaPath == "" {
		javaPath = "java"
	}

	cmd := exec.CommandContext(ctx, javaPath, "-jar", jarPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	return &execProcess{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}, nil
}

type execProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
}

func (p *execProcess) Start() error          { return p.cmd.Start() }
func (p *execProcess) Stdin() io.WriteCloser { return p.stdin }
func (p *execProcess) Stdout() io.ReadCloser { return p.stdout }
func (p *execProcess) Stderr() io.ReadCloser { return p.stderr }
func (p *execProcess) Wait() error           { return p.cmd.Wait() }
