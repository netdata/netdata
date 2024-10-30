// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package systemdunits

import (
	"fmt"
	"regexp"
	"strconv"
)

func (s *SystemdUnits) collect() (map[string]int64, error) {
	conn, err := s.getConnection()
	if err != nil {
		return nil, err
	}

	if s.systemdVersion == 0 {
		ver, err := s.getSystemdVersion(conn)
		if err != nil {
			s.closeConnection()
			return nil, err
		}
		s.systemdVersion = ver
	}

	mx := make(map[string]int64)

	if err := s.collectUnits(mx, conn); err != nil {
		s.closeConnection()
		return nil, err
	}

	if s.CollectUnitFiles && len(s.IncludeUnitFiles) > 0 {
		if err := s.collectUnitFiles(mx, conn); err != nil {
			s.closeConnection()
			return mx, err
		}
	}

	return mx, nil
}

func (s *SystemdUnits) getConnection() (systemdConnection, error) {
	if s.conn == nil {
		conn, err := s.client.connect()
		if err != nil {
			return nil, fmt.Errorf("error on creating a connection: %v", err)
		}
		s.conn = conn
	}
	return s.conn, nil
}

func (s *SystemdUnits) closeConnection() {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

var reVersion = regexp.MustCompile(`[0-9][0-9][0-9]`)

const versionProperty = "Version"

func (s *SystemdUnits) getSystemdVersion(conn systemdConnection) (int, error) {
	s.Debugf("calling function 'GetManagerProperty'")
	version, err := conn.GetManagerProperty(versionProperty)
	if err != nil {
		return 0, fmt.Errorf("error on getting '%s' manager property: %v", versionProperty, err)
	}

	s.Debugf("systemd version: %s", version)

	major := reVersion.FindString(version)
	if major == "" {
		return 0, fmt.Errorf("couldn't parse systemd version string '%s'", version)
	}

	ver, err := strconv.Atoi(major)
	if err != nil {
		return 0, fmt.Errorf("couldn't parse systemd version string '%s': %v", version, err)
	}

	return ver, nil
}
