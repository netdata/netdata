// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package systemdunits

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"
)

const transientProperty = "Transient"

const (
	// https://www.freedesktop.org/software/systemd/man/systemd.html
	unitStateActive       = "active"
	unitStateInactive     = "inactive"
	unitStateActivating   = "activating"
	unitStateDeactivating = "deactivating"
	unitStateFailed       = "failed"
)

var unitStates = []string{
	unitStateActive,
	unitStateActivating,
	unitStateFailed,
	unitStateInactive,
	unitStateDeactivating,
}

func (s *SystemdUnits) collectUnits(mx map[string]int64, conn systemdConnection) error {
	var units []dbus.UnitStatus
	var err error

	if s.systemdVersion >= 230 {
		// https://github.com/systemd/systemd/pull/3142
		units, err = s.getLoadedUnitsByPatterns(conn)
	} else {
		units, err = s.getLoadedUnits(conn)
	}
	if err != nil {
		return err
	}

	seen := make(map[string]bool)

	for _, unit := range units {
		name, typ, ok := extractUnitNameType(unit.Name)
		if !ok {
			continue
		}

		seen[unit.Name] = true

		if s.SkipTransient {
			if _, ok := s.unitTransient[unit.Name]; !ok {
				prop, err := s.getUnitTransientProperty(conn, unit.Name)
				if err != nil {
					return err
				}
				prop = strings.Trim(prop, "\"")
				s.unitTransient[unit.Name] = prop == "true"
			}
			if s.unitTransient[unit.Name] {
				continue
			}
		}

		if !s.seenUnits[unit.Name] {
			s.seenUnits[unit.Name] = true
			s.addUnitCharts(name, typ)
		}

		for _, s := range unitStates {
			mx[fmt.Sprintf("unit_%s_%s_state_%s", name, typ, s)] = 0
		}
		mx[fmt.Sprintf("unit_%s_%s_state_%s", name, typ, unit.ActiveState)] = 1
	}

	for k := range s.seenUnits {
		if !seen[k] {
			delete(s.seenUnits, k)
			if name, typ, ok := extractUnitNameType(k); ok {
				s.removeUnitCharts(name, typ)
			}
		}
	}

	for k := range s.unitTransient {
		if !seen[k] {
			delete(s.unitTransient, k)
		}
	}

	return nil
}

func (s *SystemdUnits) getLoadedUnits(conn systemdConnection) ([]dbus.UnitStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout.Duration())
	defer cancel()

	s.Debugf("calling function 'ListUnits'")
	units, err := conn.ListUnitsContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("error on ListUnits: %v", err)
	}

	for i := range units {
		units[i].Name = cleanUnitName(units[i].Name)
	}

	loaded := units[:0]
	for _, unit := range units {
		if unit.LoadState == "loaded" && s.unitSr.MatchString(unit.Name) {
			loaded = append(loaded, unit)
		}
	}

	s.Debugf("got total/loaded %d/%d units", len(units), len(loaded))

	return loaded, nil
}

func (s *SystemdUnits) getLoadedUnitsByPatterns(conn systemdConnection) ([]dbus.UnitStatus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout.Duration())
	defer cancel()

	s.Debugf("calling function 'ListUnitsByPatterns'")

	units, err := conn.ListUnitsByPatternsContext(ctx, unitStates, s.Include)
	if err != nil {
		return nil, fmt.Errorf("error on ListUnitsByPatterns: %v", err)
	}

	for i := range units {
		units[i].Name = cleanUnitName(units[i].Name)
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

func (s *SystemdUnits) getUnitTransientProperty(conn systemdConnection, unit string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout.Duration())
	defer cancel()

	s.Debugf("calling function 'GetUnitProperty' for unit '%s'", unit)

	prop, err := conn.GetUnitPropertyContext(ctx, unit, transientProperty)
	if err != nil {
		return "", fmt.Errorf("error on GetUnitProperty: %v", err)
	}

	return prop.Value.String(), nil
}

func extractUnitNameType(name string) (string, string, bool) {
	idx := strings.LastIndexByte(name, '.')
	if idx <= 0 {
		return "", "", false
	}
	return name[:idx], name[idx+1:], true
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
