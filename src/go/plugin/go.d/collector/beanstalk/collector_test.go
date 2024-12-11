// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"bufio"
	"context"
	"errors"
	"fmt"
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

	dataStats, _            = os.ReadFile("testdata/stats.txt")
	dataListTubes, _        = os.ReadFile("testdata/list-tubes.txt")
	dataStatsTubeDefault, _ = os.ReadFile("testdata/stats-tube-default.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":       dataConfigJSON,
		"dataConfigYAML":       dataConfigYAML,
		"dataStats":            dataStats,
		"dataListTubes":        dataListTubes,
		"dataStatsTubeDefault": dataStatsTubeDefault,
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
		prepare  func() (*Collector, *mockBeanstalkDaemon)
		wantFail bool
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  prepareCaseOk,
		},
		"fails on unexpected response": {
			wantFail: true,
			prepare:  prepareCaseUnexpectedResponse,
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
				t.Errorf("mock collr daemon start timed out")
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
				t.Errorf("mock collr daemon stop timed out")
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func() (*Collector, *mockBeanstalkDaemon)
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success on valid response": {
			prepare: prepareCaseOk,
			wantMetrics: map[string]int64{
				"binlog-records-migrated":            0,
				"binlog-records-written":             0,
				"cmd-bury":                           0,
				"cmd-delete":                         0,
				"cmd-ignore":                         0,
				"cmd-kick":                           0,
				"cmd-list-tube-used":                 0,
				"cmd-list-tubes":                     317,
				"cmd-list-tubes-watched":             0,
				"cmd-pause-tube":                     0,
				"cmd-peek":                           0,
				"cmd-peek-buried":                    0,
				"cmd-peek-delayed":                   0,
				"cmd-peek-ready":                     0,
				"cmd-put":                            0,
				"cmd-release":                        0,
				"cmd-reserve":                        0,
				"cmd-reserve-with-timeout":           0,
				"cmd-stats":                          23619,
				"cmd-stats-job":                      0,
				"cmd-stats-tube":                     18964,
				"cmd-touch":                          0,
				"cmd-use":                            0,
				"cmd-watch":                          0,
				"current-connections":                2,
				"current-jobs-buried":                0,
				"current-jobs-delayed":               0,
				"current-jobs-ready":                 0,
				"current-jobs-reserved":              0,
				"current-jobs-urgent":                0,
				"current-producers":                  0,
				"current-tubes":                      1,
				"current-waiting":                    0,
				"current-workers":                    0,
				"job-timeouts":                       0,
				"rusage-stime":                       3922,
				"rusage-utime":                       1602,
				"total-connections":                  72,
				"total-jobs":                         0,
				"tube_default_cmd-delete":            0,
				"tube_default_cmd-pause-tube":        0,
				"tube_default_current-jobs-buried":   0,
				"tube_default_current-jobs-delayed":  0,
				"tube_default_current-jobs-ready":    0,
				"tube_default_current-jobs-reserved": 0,
				"tube_default_current-jobs-urgent":   0,
				"tube_default_current-using":         2,
				"tube_default_current-waiting":       0,
				"tube_default_current-watching":      2,
				"tube_default_pause":                 0,
				"tube_default_pause-time-left":       0,
				"tube_default_total-jobs":            0,
				"uptime":                             105881,
			},
			wantCharts: len(statsCharts) + len(tubeChartsTmpl)*1,
		},
		"fails on unexpected response": {
			prepare:    prepareCaseUnexpectedResponse,
			wantCharts: len(statsCharts),
		},
		"fails on connection refused": {
			prepare:    prepareCaseConnectionRefused,
			wantCharts: len(statsCharts),
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
				t.Errorf("mock collr daemon start timed out")
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
				t.Errorf("mock collr daemon stop timed out")
			}
		})
	}
}

func prepareCaseOk() (*Collector, *mockBeanstalkDaemon) {
	daemon := &mockBeanstalkDaemon{
		addr:          "127.0.0.1:65001",
		started:       make(chan struct{}),
		stopped:       make(chan struct{}),
		dataStats:     dataStats,
		dataListTubes: dataListTubes,
		dataStatsTube: dataStatsTubeDefault,
	}

	collr := New()
	collr.Address = daemon.addr

	return collr, daemon
}

func prepareCaseUnexpectedResponse() (*Collector, *mockBeanstalkDaemon) {
	daemon := &mockBeanstalkDaemon{
		addr:          "127.0.0.1:65001",
		started:       make(chan struct{}),
		stopped:       make(chan struct{}),
		dataStats:     []byte("INTERNAL_ERROR\n"),
		dataListTubes: []byte("INTERNAL_ERROR\n"),
		dataStatsTube: []byte("INTERNAL_ERROR\n"),
	}

	collr := New()
	collr.Address = daemon.addr

	return collr, daemon
}

func prepareCaseConnectionRefused() (*Collector, *mockBeanstalkDaemon) {
	ch := make(chan struct{})
	close(ch)
	daemon := &mockBeanstalkDaemon{
		addr:      "127.0.0.1:65001",
		dontStart: true,
		started:   ch,
		stopped:   ch,
	}

	collr := New()
	collr.Address = daemon.addr

	return collr, daemon
}

type mockBeanstalkDaemon struct {
	addr      string
	srv       net.Listener
	started   chan struct{}
	stopped   chan struct{}
	dontStart bool

	dataStats     []byte
	dataListTubes []byte
	dataStatsTube []byte
}

func (m *mockBeanstalkDaemon) Run() error {
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

func (m *mockBeanstalkDaemon) Close() error {
	if m.srv != nil {
		err := m.srv.Close()
		m.srv = nil
		return err
	}
	return nil
}

func (m *mockBeanstalkDaemon) handleConnections() error {
	conn, err := m.srv.Accept()
	if err != nil || conn == nil {
		return errors.New("could not accept connection")
	}
	return m.handleConnection(conn)
}

func (m *mockBeanstalkDaemon) handleConnection(conn net.Conn) error {
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
			return nil
		case cmdStats:
			_, err = rw.Write(m.dataStats)
		case cmdListTubes:
			_, err = rw.Write(m.dataListTubes)
		case cmdStatsTube:
			if param == "default" {
				_, err = rw.Write(m.dataStatsTube)
			} else {
				_, err = rw.WriteString("NOT_FOUND\n")
			}
		default:
			return fmt.Errorf("unexpected command: %s", line)
		}
		_ = rw.Flush()
		if err != nil {
			return err
		}
	}
}
