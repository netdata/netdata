// SPDX-License-Identifier: GPL-3.0-or-later

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

func (s *Runit) servicesCli() (map[string]*ServiceStatus, error) {
	ms := make(map[string]*ServiceStatus, hintServices)

	out, err := s.exec.StatusAll(s.Config.Dir)
	if err != nil {
		return nil, err
	}

	buf := bytes.NewBuffer(out)
	for {
		line, err := buf.ReadBytes('\n')
		match := reSvStatus.FindSubmatch(line)

		switch {
		case match != nil:
			match = match[1:]
			matchSv := match[:offsetLog]
			matchLog := match[offsetLog:]
			name := strings.TrimPrefix(string(matchSv[idxName]), s.Config.Dir+"/")
			ms[name] = parseServiceStatus(matchSv)
			if string(matchLog[idxName]) == "log" {
				ms[name+"/log"] = parseServiceStatus(matchLog)
			}
		case bytes.HasPrefix(line, pfxSvFail):
		case bytes.HasPrefix(line, pfxSvWarn):
		case len(line) > 0:
			s.Warningf("failed to parse sv status: %q", string(line))
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
