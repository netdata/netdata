// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"

	"gopkg.in/yaml.v2"
)

type beanstalkConn interface {
	connect() error
	disconnect() error
	queryStats() (*beanstalkdStats, error)
	queryListTubes() ([]string, error)
	queryStatsTube(string) (*tubeStats, error)
}

// https://github.com/beanstalkd/beanstalkd/blob/91c54fc05dc759ef27459ce4383934e1a4f2fb4b/doc/protocol.txt#L553
type beanstalkdStats struct {
	CurrentJobsUrgent     int64   `yaml:"current-jobs-urgent" stm:"current-jobs-urgent"`
	CurrentJobsReady      int64   `yaml:"current-jobs-ready" stm:"current-jobs-ready"`
	CurrentJobsReserved   int64   `yaml:"current-jobs-reserved" stm:"current-jobs-reserved"`
	CurrentJobsDelayed    int64   `yaml:"current-jobs-delayed" stm:"current-jobs-delayed"`
	CurrentJobsBuried     int64   `yaml:"current-jobs-buried" stm:"current-jobs-buried"`
	CmdPut                int64   `yaml:"cmd-put" stm:"cmd-put"`
	CmdPeek               int64   `yaml:"cmd-peek" stm:"cmd-peek"`
	CmdPeekReady          int64   `yaml:"cmd-peek-ready" stm:"cmd-peek-ready"`
	CmdPeekDelayed        int64   `yaml:"cmd-peek-delayed" stm:"cmd-peek-delayed"`
	CmdPeekBuried         int64   `yaml:"cmd-peek-buried" stm:"cmd-peek-buried"`
	CmdReserve            int64   `yaml:"cmd-reserve" stm:"cmd-reserve"`
	CmdReserveWithTimeout int64   `yaml:"cmd-reserve-with-timeout" stm:"cmd-reserve-with-timeout"`
	CmdTouch              int64   `yaml:"cmd-touch" stm:"cmd-touch"`
	CmdUse                int64   `yaml:"cmd-use" stm:"cmd-use"`
	CmdWatch              int64   `yaml:"cmd-watch" stm:"cmd-watch"`
	CmdIgnore             int64   `yaml:"cmd-ignore" stm:"cmd-ignore"`
	CmdDelete             int64   `yaml:"cmd-delete" stm:"cmd-delete"`
	CmdRelease            int64   `yaml:"cmd-release" stm:"cmd-release"`
	CmdBury               int64   `yaml:"cmd-bury" stm:"cmd-bury"`
	CmdKick               int64   `yaml:"cmd-kick" stm:"cmd-kick"`
	CmdStats              int64   `yaml:"cmd-stats" stm:"cmd-stats"`
	CmdStatsJob           int64   `yaml:"cmd-stats-job" stm:"cmd-stats-job"`
	CmdStatsTube          int64   `yaml:"cmd-stats-tube" stm:"cmd-stats-tube"`
	CmdListTubes          int64   `yaml:"cmd-list-tubes" stm:"cmd-list-tubes"`
	CmdListTubeUsed       int64   `yaml:"cmd-list-tube-used" stm:"cmd-list-tube-used"`
	CmdListTubesWatched   int64   `yaml:"cmd-list-tubes-watched" stm:"cmd-list-tubes-watched"`
	CmdPauseTube          int64   `yaml:"cmd-pause-tube" stm:"cmd-pause-tube"`
	JobTimeouts           int64   `yaml:"job-timeouts" stm:"job-timeouts"`
	TotalJobs             int64   `yaml:"total-jobs" stm:"total-jobs"`
	CurrentTubes          int64   `yaml:"current-tubes" stm:"current-tubes"`
	CurrentConnections    int64   `yaml:"current-connections" stm:"current-connections"`
	CurrentProducers      int64   `yaml:"current-producers" stm:"current-producers"`
	CurrentWorkers        int64   `yaml:"current-workers" stm:"current-workers"`
	CurrentWaiting        int64   `yaml:"current-waiting" stm:"current-waiting"`
	TotalConnections      int64   `yaml:"total-connections" stm:"total-connections"`
	RusageUtime           float64 `yaml:"rusage-utime" stm:"rusage-utime,1000,1"`
	RusageStime           float64 `yaml:"rusage-stime" stm:"rusage-stime,1000,1"`
	Uptime                int64   `yaml:"uptime" stm:"uptime"`
	BinlogRecordsWritten  int64   `yaml:"binlog-records-written" stm:"binlog-records-written"`
	BinlogRecordsMigrated int64   `yaml:"binlog-records-migrated" stm:"binlog-records-migrated"`
}

// https://github.com/beanstalkd/beanstalkd/blob/91c54fc05dc759ef27459ce4383934e1a4f2fb4b/doc/protocol.txt#L497
type tubeStats struct {
	Name                string  `yaml:"name"`
	CurrentJobsUrgent   int64   `yaml:"current-jobs-urgent" stm:"current-jobs-urgent"`
	CurrentJobsReady    int64   `yaml:"current-jobs-ready" stm:"current-jobs-ready"`
	CurrentJobsReserved int64   `yaml:"current-jobs-reserved" stm:"current-jobs-reserved"`
	CurrentJobsDelayed  int64   `yaml:"current-jobs-delayed" stm:"current-jobs-delayed"`
	CurrentJobsBuried   int64   `yaml:"current-jobs-buried" stm:"current-jobs-buried"`
	TotalJobs           int64   `yaml:"total-jobs" stm:"total-jobs"`
	CurrentUsing        int64   `yaml:"current-using" stm:"current-using"`
	CurrentWaiting      int64   `yaml:"current-waiting" stm:"current-waiting"`
	CurrentWatching     int64   `yaml:"current-watching" stm:"current-watching"`
	Pause               float64 `yaml:"pause" stm:"pause"`
	CmdDelete           int64   `yaml:"cmd-delete" stm:"cmd-delete"`
	CmdPauseTube        int64   `yaml:"cmd-pause-tube" stm:"cmd-pause-tube"`
	PauseTimeLeft       float64 `yaml:"pause-time-left" stm:"pause-time-left"`
}

func newBeanstalkConn(conf Config, log *logger.Logger) beanstalkConn {
	return &beanstalkClient{
		Logger: log,
		client: socket.New(socket.Config{
			Address:      conf.Address,
			Timeout:      conf.Timeout.Duration(),
			MaxReadLines: 2000,
			TLSConf:      nil,
		}),
	}
}

const (
	cmdQuit      = "quit"
	cmdStats     = "stats"
	cmdListTubes = "list-tubes"
	cmdStatsTube = "stats-tube"
)

type beanstalkClient struct {
	*logger.Logger

	client socket.Client
}

func (c *beanstalkClient) connect() error {
	return c.client.Connect()
}

func (c *beanstalkClient) disconnect() error {
	_, _, _ = c.query(cmdQuit)
	return c.client.Disconnect()
}

func (c *beanstalkClient) queryStats() (*beanstalkdStats, error) {
	cmd := cmdStats

	resp, data, err := c.query(cmd)
	if err != nil {
		return nil, err
	}
	if resp != "OK" {
		return nil, fmt.Errorf("command '%s' bad response: %s", cmd, resp)
	}

	var stats beanstalkdStats

	if err := yaml.Unmarshal(data, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (c *beanstalkClient) queryListTubes() ([]string, error) {
	cmd := cmdListTubes

	resp, data, err := c.query(cmd)
	if err != nil {
		return nil, err
	}
	if resp != "OK" {
		return nil, fmt.Errorf("command '%s' bad response: %s", cmd, resp)
	}

	var tubes []string

	if err := yaml.Unmarshal(data, &tubes); err != nil {
		return nil, err
	}

	return tubes, nil
}

func (c *beanstalkClient) queryStatsTube(tubeName string) (*tubeStats, error) {
	cmd := fmt.Sprintf("%s %s", cmdStatsTube, tubeName)

	resp, data, err := c.query(cmd)
	if err != nil {
		return nil, err
	}
	if resp == "NOT_FOUND" {
		return nil, nil
	}
	if resp != "OK" {
		return nil, fmt.Errorf("command '%s' bad response: %s", cmd, resp)
	}

	var stats tubeStats
	if err := yaml.Unmarshal(data, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (c *beanstalkClient) query(command string) (string, []byte, error) {
	c.Debugf("executing command: %s", command)

	var (
		resp   string
		body   []byte
		length int
		err    error
	)

	if err := c.client.Command(command+"\r\n", func(line []byte) (bool, error) {
		if resp == "" {
			s := string(line)
			c.Debugf("command '%s' response: '%s'", command, s)

			resp, length, err = parseResponseLine(s)
			if err != nil {
				return false, fmt.Errorf("command '%s' line '%s': %v", command, s, err)
			}

			return resp == "OK", nil
		}

		body = append(body, line...)
		body = append(body, '\n')

		return len(body) < length, nil
	}); err != nil {
		return "", nil, fmt.Errorf("command '%s': %v", command, err)
	}

	return resp, body, nil
}

func parseResponseLine(line string) (string, int, error) {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", 0, errors.New("empty response")
	}

	resp := parts[0]

	if resp != "OK" {
		return resp, 0, nil
	}

	if len(parts) < 2 {
		return "", 0, errors.New("missing bytes count")
	}

	length, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, errors.New("invalid bytes count")
	}

	return resp, length, nil
}
