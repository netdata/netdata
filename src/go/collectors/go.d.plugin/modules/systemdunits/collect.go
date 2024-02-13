// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"
)

const (
	// https://www.freedesktop.org/software/systemd/man/systemd.html
	unitStateActive       = "active"
	unitStateInactive     = "inactive"
	unitStateActivating   = "activating"
	unitStateDeactivating = "deactivating"
	unitStateFailed       = "failed"

	// https://www.freedesktop.org/software/systemd/man/systemd.html
	unitTypeService   = "service"
	unitTypeSocket    = "socket"
	unitTypeTarget    = "target"
	unitTypePath      = "path"
	unitTypeDevice    = "device"
	unitTypeMount     = "mount"
	unitTypeAutomount = "automount"
	unitTypeSwap      = "swap"
	unitTypeTimer     = "timer"
	unitTypeScope     = "scope"
	unitTypeSlice     = "slice"
)

var (
	unitStates = []string{
		unitStateActive,
		unitStateActivating,
		unitStateFailed,
		unitStateInactive,
		unitStateDeactivating,
	}
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

	var units []dbus.UnitStatus
	if s.systemdVersion >= 230 {
		// https://github.com/systemd/systemd/pull/3142
		units, err = s.getLoadedUnitsByPatterns(conn)
	} else {
		units, err = s.getLoadedUnits(conn)
	}
	if err != nil {
		s.closeConnection()
		return nil, err
	}

	if len(units) == 0 {
		return nil, nil
	}

	mx := make(map[string]int64)
	s.collectUnitsStates(mx, units)

	return mx, nil
}

func (s *SystemdUnits) collectUnitsStates(mx map[string]int64, units []dbus.UnitStatus) {
	for _, unit := range units {
		name, typ := extractUnitNameType(cleanUnitName(unit.Name))
		if name == "" || typ == "" {
			continue
		}

		if !s.units[unit.Name] {
			s.units[unit.Name] = true
			s.addUnitToCharts(name, typ)
		}

		for _, s := range unitStates {
			mx[fmt.Sprintf("unit_%s_%s_state_%s", name, typ, s)] = 0
		}
		mx[fmt.Sprintf("unit_%s_%s_state_%s", name, typ, unit.ActiveState)] = 1
	}
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

func (s *SystemdUnits) getLoadedUnits(conn systemdConnection) ([]dbus.UnitStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout.Duration)
	defer cancel()

	s.Debugf("calling function 'ListUnits'")
	units, err := conn.ListUnitsContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on ListUnits: %v", err)
	}

	loaded := units[:0]
	for _, unit := range units {
		if unit.LoadState == "loaded" && s.sr.MatchString(unit.Name) {
			loaded = append(loaded, unit)
		}
	}
	s.Debugf("got total/loaded %d/%d units", len(units), len(loaded))

	return loaded, nil
}

func (s *SystemdUnits) getLoadedUnitsByPatterns(conn systemdConnection) ([]dbus.UnitStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout.Duration)
	defer cancel()

	s.Debugf("calling function 'ListUnitsByPatterns'")

	units, err := conn.ListUnitsByPatternsContext(ctx, unitStates, s.Include)
	if err != nil {
		return nil, fmt.Errorf("error on ListUnitsByPatterns: %v", err)
	}

	loaded := units[:0]
	for _, unit := range units {
		if unit.LoadState == "loaded" {
			loaded = append(loaded, unit)
		}
	}
	s.Debugf("got total/loaded %d/%d units", len(units), len(loaded))

	return loaded, nil
}

func extractUnitNameType(name string) (string, string) {
	idx := strings.LastIndexByte(name, '.')
	if idx <= 0 {
		return "", ""
	}
	return name[:idx], name[idx+1:]
}

func cleanUnitName(name string) string {
	// dev-disk-by\x2duuid-DE44\x2dCEE0.device => dev-disk-by-uuid-DE44-CEE0.device
	if strings.IndexByte(name, '\\') == -1 {
		return name
	}
	v, err := strconv.Unquote("\"" + name + "\"")
	if err != nil {
		return name
	}
	return v
}
