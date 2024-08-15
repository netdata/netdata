// SPDX-License-Identifier: GPL-3.0-or-later

package monit

// status_xml(): https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/http/xml.c#lines-631
type monitStatus struct {
	Server   *statusServer        `xml:"server"`
	Services []statusServiceCheck `xml:"service"`
}

type statusServer struct {
	ID            string `xml:"id"`
	Version       string `xml:"version"`
	Uptime        int64  `xml:"uptime"`
	LocalHostname string `xml:"localhostname"`
}

// status_service(): https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/http/xml.c#lines-196
// struct Service_T: https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/monit.h#lines-1212
type statusServiceCheck struct {
	Type string `xml:"type,attr"`
	Name string `xml:"name"`

	Status int `xml:"status"` // Error flags bitmap

	// https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/monit.h#lines-269
	MonitoringStatus int `xml:"monitor"`

	// https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/monit.h#lines-254
	MonitorMode int `xml:"monitormode"`

	// https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/monit.h#lines-261
	OnReboot int `xml:"onreboot"`

	// https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/monit.h#lines-248
	PendingAction int `xml:"pendingaction"`
}

func (s *statusServiceCheck) id() string {
	return s.svcType() + ":" + s.Name
}

func (s *statusServiceCheck) svcType() string {
	// See enum Service_Type https://bitbucket.org/tildeslash/monit/src/master/src/monit.h

	switch s.Type {
	case "0":
		return "filesystem"
	case "1":
		return "directory"
	case "2":
		return "file"
	case "3":
		return "process"
	case "4":
		return "host"
	case "5":
		return "system"
	case "6":
		return "fifo"
	case "7":
		return "program"
	case "8":
		return "network"
	default:
		return "unknown"
	}
}

func (s *statusServiceCheck) status() string {
	// https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/http/cervlet.c#lines-2866

	switch st := s.monitoringStatus(); st {
	case "not_monitored", "initializing":
		return st
	default:
		if s.Status != 0 {
			return "error"
		}
		return "ok"
	}
}

func (s *statusServiceCheck) monitoringStatus() string {
	switch s.MonitoringStatus {
	case 0:
		return "not_monitored"
	case 1:
		return "monitored"
	case 2:
		return "initializing"
	case 4:
		return "waiting"
	default:
		return "unknown"
	}
}

func (s *statusServiceCheck) monitorMode() string {
	switch s.MonitorMode {
	case 0:
		return "active"
	case 1:
		return "passive"
	default:
		return "unknown"
	}
}

func (s *statusServiceCheck) onReboot() string {
	switch s.OnReboot {
	case 0:
		return "start"
	case 1:
		return "no_start"
	default:
		return "unknown"
	}
}

func (s *statusServiceCheck) pendingAction() string {
	switch s.PendingAction {
	case 0:
		return "ignored"
	case 1:
		return "alert"
	case 2:
		return "restart"
	case 3:
		return "stop"
	case 4:
		return "exec"
	case 5:
		return "unmonitor"
	case 6:
		return "start"
	case 7:
		return "monitor"
	default:
		return "unknown"
	}
}

func (s *statusServiceCheck) hasServiceStatus() bool {
	// https://bitbucket.org/tildeslash/monit/src/5467d37d70c3c63c5760cddb93831bde4e17c14b/src/util.c#lines-1721

	const eventNonExist = 512
	const eventData = 2048

	return s.monitoringStatus() == "monitored" &&
		!(s.Status&eventNonExist != 0) &&
		!(s.Status&eventData != 0)
}
