// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux
// +build linux

package systemdunits

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

// https://github.com/systemd/systemd/blob/3d320785c4bbba74459096b07e85a79c4f0cdffb/src/shared/install.c#L3785
// see "is-enabled" in https://www.man7.org/linux/man-pages/man1/systemctl.1.html
var unitFileStates = []string{
	"enabled",
	"enabled-runtime",
	"linked",
	"linked-runtime",
	"alias",
	"masked",
	"masked-runtime",
	"static",
	"disabled",
	"indirect",
	"generated",
	"transient",
	"bad",
}

func (s *SystemdUnits) collectUnitFiles(mx map[string]int64, conn systemdConnection) error {
	if s.systemdVersion < 230 {
		return nil
	}

	if now := time.Now(); now.After(s.lastListUnitFilesTime.Add(s.CollectUnitFilesEvery.Duration())) {
		unitFiles, err := s.getUnitFilesByPatterns(conn)
		if err != nil {
			return err
		}
		s.lastListUnitFilesTime = now
		s.cachedUnitFiles = unitFiles
	}

	seen := make(map[string]bool)

	for _, unitFile := range s.cachedUnitFiles {
		seen[unitFile.Path] = true

		if !s.seenUnitFiles[unitFile.Path] {
			s.seenUnitFiles[unitFile.Path] = true
			s.addUnitFileCharts(unitFile.Path)
		}

		px := fmt.Sprintf("unit_file_%s_state_", unitFile.Path)
		for _, st := range unitFileStates {
			mx[px+st] = 0
		}
		mx[px+strings.ToLower(unitFile.Type)] = 1
	}

	for k := range s.seenUnitFiles {
		if !seen[k] {
			delete(s.seenUnitFiles, k)
			s.removeUnitFileCharts(k)
		}
	}

	return nil
}

func (s *SystemdUnits) getUnitFilesByPatterns(conn systemdConnection) ([]dbus.UnitFile, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.Timeout.Duration())
	defer cancel()

	s.Debugf("calling function 'ListUnitFilesByPatterns'")

	unitFiles, err := conn.ListUnitFilesByPatternsContext(ctx, nil, s.IncludeUnitFiles)
	if err != nil {
		return nil, fmt.Errorf("error on ListUnitFilesByPatterns: %v", err)
	}

	for i := range unitFiles {
		unitFiles[i].Path = cleanUnitName(unitFiles[i].Path)
	}

	s.Debugf("got %d unit files", len(unitFiles))

	return unitFiles, nil
}
