// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const hintServices = 64 // Large enough amount of services for most service directories.

type ServiceStatus struct {
	StateDuration time.Duration
	Paused        bool
	WantUp        bool
	State         ServiceState
	Enabled       bool
}

type ServiceState uint8

const (
	ServiceStateDown   ServiceState = 0
	ServiceStateRun    ServiceState = 1
	ServiceStateFinish ServiceState = 2
)

// Constants.
var (
	tokenUp    = []byte("up")
	tokenRun   = []byte("run")
	tokenDown  = []byte("down")
	pfxSvFail  = []byte("fail:")
	pfxSvWarn  = []byte("warning:")
	elemSvInfo = `(?:\(pid \d+\) )?` +
		`(\d+)s` + // uptime/downtime
		`(?:, normally (up|down))?` +
		`(, paused)?` +
		`(?:, want (up|down))?` +
		`(, got TERM)?`
	reSvStatus = regexp.MustCompile(`^` +
		`(down|run|finish): ` +
		`(.*?): ` + // service directory
		elemSvInfo +
		`(?:; warning:.*|; (down|run|finish): (log): ` + elemSvInfo + `)?` +
		`\n$`)
)

// Leftovers of a status line when sv cannot stat a `down` file (see servicesCli).
var (
	lineSvNoStatus   = []byte("\n")
	pfxSvNoStatus    = []byte("; ")
	sfxSvNoLogStatus = []byte("; \n")
)

const (
	idxState = iota
	idxName
	idxDuration
	idxNormally
	idxPaused
	idxWant
	idxTERM
	offsetLog
)

func (c *Collector) servicesCli() (map[string]*ServiceStatus, error) {
	ms := make(map[string]*ServiceStatus, hintServices)

	out, err := c.exec.StatusAll(c.Dir)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(out)
	for {
		line, err := buf.ReadBytes('\n')
		// When sv cannot stat the `down` file of a service (e.g. a symlink loop),
		// it warns on stderr only, does not count it in its exit code,
		// and prints the status line without the status of that service.
		// The leftover is the only reliable trace of the problem:
		// an empty line, or the "; <log status>" tail when the service has a log service.
		// For the `log/down` file the line ends with "; " instead of the log service status,
		// while the status of the service itself is valid.
		noLogStatus := bytes.HasSuffix(line, sfxSvNoLogStatus)
		if noLogStatus {
			line = append(bytes.TrimSuffix(line, sfxSvNoLogStatus), '\n')
		}
		match := reSvStatus.FindSubmatch(line)

		switch {
		case match != nil:
			match = match[1:]
			matchSv := match[:offsetLog]
			matchLog := match[offsetLog:]
			name := strings.TrimPrefix(string(matchSv[idxName]), c.Dir+"/")
			ms[name] = parseServiceStatus(matchSv)
			if string(matchLog[idxName]) == "log" {
				ms[name+"/log"] = parseServiceStatus(matchLog)
			}
			if noLogStatus {
				c.Warningf("sv could not stat the 'log/down' file of service '%s', its log status is unknown", name)
			}
		case bytes.HasPrefix(line, pfxSvFail):
		case bytes.HasPrefix(line, pfxSvWarn):
		case bytes.Equal(line, lineSvNoStatus) || bytes.HasPrefix(line, pfxSvNoStatus):
			c.Warningf("sv could not stat the 'down' file of a service, its status is unknown: %q", string(line))
		case len(line) > 0:
			c.Warningf("failed to parse sv status: %q", string(line))
		}

		if err != nil {
			break
		}
	}

	return ms, nil
}

func parseServiceStatus(match [][]byte) *ServiceStatus {
	status := &ServiceStatus{
		StateDuration: parseServiceDuration(match[idxDuration]),
		Paused:        len(match[idxPaused]) != 0,
		WantUp:        bytes.Equal(match[idxWant], tokenUp),
		State:         parseServiceState(match[idxState]),
	}
	if status.State == ServiceStateDown {
		status.Enabled = bytes.Equal(match[idxNormally], tokenUp)
	} else {
		status.Enabled = !bytes.Equal(match[idxNormally], tokenDown)
	}
	return status
}

func parseServiceState(state []byte) ServiceState {
	switch {
	case bytes.Equal(state, tokenDown):
		return ServiceStateDown
	case bytes.Equal(state, tokenRun):
		return ServiceStateRun
	default:
		return ServiceStateFinish
	}
}

func parseServiceDuration(seconds []byte) time.Duration {
	sec, _ := strconv.Atoi(string(seconds))
	return time.Duration(sec) * time.Second
}
