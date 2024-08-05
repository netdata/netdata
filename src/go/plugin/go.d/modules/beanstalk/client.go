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

// https://github.com/beanstalkd/beanstalkd/blob/91c54fc05dc759ef27459ce4383934e1a4f2fb4b/doc/protocol.txt#L553
type beanstalkdStats struct {
	CurrentJobsUrgent     int64   `yaml:"current-jobs-urgent"`
	CurrentJobsReady      int64   `yaml:"current-jobs-ready"`
	CurrentJobsReserved   int64   `yaml:"current-jobs-reserved"`
	CurrentJobsDelayed    int64   `yaml:"current-jobs-delayed"`
	CurrentJobsBuried     int64   `yaml:"current-jobs-buried"`
	CmdPut                int64   `yaml:"cmd-put"`
	CmdPeek               int64   `yaml:"cmd-peek"`
	CmdPeekReady          int64   `yaml:"cmd-peek-ready"`
	CmdPeekDelayed        int64   `yaml:"cmd-peek-delayed"`
	CmdPeekBuried         int64   `yaml:"cmd-peek-buried"`
	CmdReserve            int64   `yaml:"cmd-reserve"`
	CmdReserveWithTimeout int64   `yaml:"cmd-reserve-with-timeout"`
	CmdTouch              int64   `yaml:"cmd-touch"`
	CmdUse                int64   `yaml:"cmd-use"`
	CmdWatch              int64   `yaml:"cmd-watch"`
	CmdIgnore             int64   `yaml:"cmd-ignore"`
	CmdDelete             int64   `yaml:"cmd-delete"`
	CmdRelease            int64   `yaml:"cmd-release"`
	CmdBury               int64   `yaml:"cmd-bury"`
	CmdKick               int64   `yaml:"cmd-kick"`
	CmdStats              int64   `yaml:"cmd-stats"`
	CmdStatsJob           int64   `yaml:"cmd-stats-job"`
	CmdStatsTube          int64   `yaml:"cmd-stats-tube"`
	CmdListTubes          int64   `yaml:"cmd-list-tubes"`
	CmdListTubeUsed       int64   `yaml:"cmd-list-tube-used"`
	CmdListTubesWatched   int64   `yaml:"cmd-list-tubes-watched"`
	CmdPauseTube          int64   `yaml:"cmd-pause-tube"`
	JobTimeouts           int64   `yaml:"job-timeouts"`
	TotalJobs             int64   `yaml:"total-jobs"`
	MaxJobSize            int64   `yaml:"max-job-size"`
	CurrentTubes          int64   `yaml:"current-tubes"`
	CurrentConnections    int64   `yaml:"current-connections"`
	CurrentProducers      int64   `yaml:"current-producers"`
	CurrentWorkers        int64   `yaml:"current-workers"`
	CurrentWaiting        int64   `yaml:"current-waiting"`
	TotalConnections      int64   `yaml:"total-connections"`
	Pid                   int64   `yaml:"pid"`
	Version               string  `yaml:"version"`
	RusageUtime           float64 `yaml:"rusage-utime"`
	RusageStime           float64 `yaml:"rusage-stime"`
	Uptime                int64   `yaml:"uptime"`
	BinlogOldestIndex     int64   `yaml:"binlog-oldest-index"`
	BinlogCurrentIndex    int64   `yaml:"binlog-current-index"`
	BinlogMaxSize         int64   `yaml:"binlog-max-size"`
	BinlogRecordsWritten  int64   `yaml:"binlog-records-written"`
	BinlogRecordsMigrated int64   `yaml:"binlog-records-migrated"`
	Draining              bool    `yaml:"draining"`
	ID                    string  `yaml:"id"`
	Hostname              string  `yaml:"hostname"`
	OS                    string  `yaml:"os"`
	Platform              string  `yaml:"platform"`
}

// https://github.com/beanstalkd/beanstalkd/blob/91c54fc05dc759ef27459ce4383934e1a4f2fb4b/doc/protocol.txt#L497
type tubeStats struct {
	Name                string `yaml:"name"`
	CurrentJobsUrgent   int64  `yaml:"current-jobs-urgent"`
	CurrentJobsReady    int64  `yaml:"current-jobs-ready"`
	CurrentJobsReserved int64  `yaml:"current-jobs-reserved"`
	CurrentJobsDelayed  int64  `yaml:"current-jobs-delayed"`
	CurrentJobsBuried   int64  `yaml:"current-jobs-buried"`
	TotalJobs           int64  `yaml:"total-jobs"`
	CurrentUsing        int64  `yaml:"current-using"`
	CurrentWaiting      int64  `yaml:"current-waiting"`
	CurrentWatching     int64  `yaml:"current-watching"`
	Pause               int64  `yaml:"pause"`
	CmdDelete           int64  `yaml:"cmd-delete"`
	CmdPauseTube        int64  `yaml:"cmd-pause-tube"`
	PauseTimeLeft       int64  `yaml:"pause-time-left"`
}

type beanstalkClient struct {
	*logger.Logger

	client socket.Client
}

func newBeanstalkConn(conf Config) beanstalkConn {
	return &beanstalkClient{
		client: socket.New(socket.Config{
			Address:        conf.Address,
			ConnectTimeout: conf.Timeout.Duration(),
			ReadTimeout:    conf.Timeout.Duration(),
			WriteTimeout:   conf.Timeout.Duration(),
			TLSConf:        nil,
		}),
	}
}

func (c *beanstalkClient) connect() error {
	return c.client.Connect()
}

func (c *beanstalkClient) disconnect() error {
	cmd := "quit"
	_, _, _ = c.query(cmd)
	return c.client.Disconnect()
}

func (c *beanstalkClient) queryStats() (*beanstalkdStats, error) {
	cmd := "stats"

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
	cmd := "list-tubes"

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
	cmd := fmt.Sprintf("stats-tube %s", tubeName)

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
	var resp string
	var length int
	var body []byte
	var err error

	c.Debugf("executing command: %s", command)

	err = c.client.Command(command+"\r\n", func(line []byte) bool {
		if resp == "" {
			s := string(line)
			c.Debugf("command '%s' response '%s'", command, s)

			resp, length, err = parseResponseLine(s)
			if err != nil {
				err = fmt.Errorf("command '%s' line '%s': %v", command, s, err)
			}
			return err == nil && resp == "OK"
		}

		body = append(body, line...)
		body = append(body, '\n')

		return len(body) < length
	})
	if err != nil {
		return "", nil, err
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
