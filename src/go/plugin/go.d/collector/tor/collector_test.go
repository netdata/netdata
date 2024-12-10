// SPDX-License-Identifier: GPL-3.0-or-later

package tor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success with default config": {
			wantFail: false,
			config:   New().Config,
		},
		"fails if address not set": {
			wantFail: true,
			config: func() Config {
				conf := New().Config
				conf.Address = ""
				return conf
			}(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (*Collector, *mockTorDaemon)
		wantFail bool
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  prepareCaseOk,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  prepareCaseConnectionRefused,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, daemon := test.prepare()

			defer func() {
				assert.NoError(t, daemon.Close(), "daemon.Close()")
			}()
			go func() {
				assert.NoError(t, daemon.Run(), "daemon.Run()")
			}()

			select {
			case <-daemon.started:
			case <-time.After(time.Second * 3):
				t.Errorf("mock tor daemon start timed out")
			}

			require.NoError(t, collr.Init(context.Background()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}

			collr.Cleanup(context.Background())

			select {
			case <-daemon.stopped:
			case <-time.After(time.Second * 3):
				t.Errorf("mock tor daemon stop timed out")
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func() (*Collector, *mockTorDaemon)
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success on valid response": {
			prepare:    prepareCaseOk,
			wantCharts: len(charts),
			wantMetrics: map[string]int64{
				"traffic/read":    100,
				"traffic/written": 100,
				"uptime":          100,
			},
		},
		"fails on connection refused": {
			prepare:    prepareCaseConnectionRefused,
			wantCharts: len(charts),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, daemon := test.prepare()

			defer func() {
				assert.NoError(t, daemon.Close(), "daemon.Close()")
			}()
			go func() {
				assert.NoError(t, daemon.Run(), "daemon.Run()")
			}()

			select {
			case <-daemon.started:
			case <-time.After(time.Second * 3):
				t.Errorf("mock tor daemon start timed out")
			}

			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			assert.Equal(t, test.wantCharts, len(*collr.Charts()), "want charts")

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}

			collr.Cleanup(context.Background())

			select {
			case <-daemon.stopped:
			case <-time.After(time.Second * 3):
				t.Errorf("mock tordaemon stop timed out")
			}
		})
	}
}

func prepareCaseOk() (*Collector, *mockTorDaemon) {
	daemon := &mockTorDaemon{
		addr:    "127.0.0.1:65001",
		started: make(chan struct{}),
		stopped: make(chan struct{}),
	}

	collr := New()
	collr.Address = daemon.addr

	return collr, daemon
}

func prepareCaseConnectionRefused() (*Collector, *mockTorDaemon) {
	ch := make(chan struct{})
	close(ch)

	daemon := &mockTorDaemon{
		addr:      "127.0.0.1:65001",
		dontStart: true,
		started:   ch,
		stopped:   ch,
	}

	collr := New()
	collr.Address = daemon.addr

	return collr, daemon
}

type mockTorDaemon struct {
	addr          string
	srv           net.Listener
	started       chan struct{}
	stopped       chan struct{}
	dontStart     bool
	authenticated bool
}

func (m *mockTorDaemon) Run() error {
	if m.dontStart {
		return nil
	}

	srv, err := net.Listen("tcp", m.addr)
	if err != nil {
		return err
	}

	m.srv = srv

	close(m.started)
	defer close(m.stopped)

	return m.handleConnections()
}

func (m *mockTorDaemon) Close() error {
	if m.srv != nil {
		err := m.srv.Close()
		m.srv = nil
		return err
	}
	return nil
}

func (m *mockTorDaemon) handleConnections() error {
	conn, err := m.srv.Accept()
	if err != nil || conn == nil {
		return errors.New("could not accept connection")
	}
	return m.handleConnection(conn)
}

func (m *mockTorDaemon) handleConnection(conn net.Conn) error {
	defer func() { _ = conn.Close() }()

	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	var line string
	var err error

	for {
		if line, err = rw.ReadString('\n'); err != nil {
			return fmt.Errorf("error reading from connection: %v", err)
		}

		line = strings.TrimSpace(line)

		cmd, param, _ := strings.Cut(line, " ")

		switch cmd {
		case cmdQuit:
			return m.handleQuit(conn)
		case cmdAuthenticate:
			err = m.handleAuthenticate(conn)
		case cmdGetInfo:
			err = m.handleGetInfo(conn, param)
		default:
			s := fmt.Sprintf("510 Unrecognized command \"%s\"\n", cmd)
			_, _ = rw.WriteString(s)
			return fmt.Errorf("unexpected command: %s", line)
		}

		_ = rw.Flush()

		if err != nil {
			return err
		}
	}
}

func (m *mockTorDaemon) handleQuit(conn io.Writer) error {
	_, err := conn.Write([]byte("250 closing connection\n"))
	return err
}

func (m *mockTorDaemon) handleAuthenticate(conn io.Writer) error {
	m.authenticated = true
	_, err := conn.Write([]byte("250 OK\n"))
	return err
}

func (m *mockTorDaemon) handleGetInfo(conn io.Writer, keywords string) error {
	if !m.authenticated {
		_, _ = conn.Write([]byte("514 Authentication required\n"))
		return errors.New("authentication required")
	}

	keywords = strings.Trim(keywords, "\"")

	for _, k := range strings.Fields(keywords) {
		s := fmt.Sprintf("250-%s=%d\n", k, 100)

		if _, err := conn.Write([]byte(s)); err != nil {
			return err
		}
	}

	_, err := conn.Write([]byte("250 OK\n"))
	return err
}
