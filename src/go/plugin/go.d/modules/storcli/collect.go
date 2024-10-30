// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package storcli

import "fmt"

const (
	driverNameMegaraid = "megaraid_sas"
	driverNameSas      = "mpt3sas"
)

func (s *StorCli) collect() (map[string]int64, error) {
	cntrlResp, err := s.queryControllersInfo()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	driver := cntrlResp.Controllers[0].ResponseData.Version.DriverName

	switch driver {
	case driverNameMegaraid:
		if err := s.collectMegaraidControllersInfo(mx, cntrlResp); err != nil {
			return nil, fmt.Errorf("failed to collect megaraid controller info: %s", err)
		}
		if len(cntrlResp.Controllers[0].ResponseData.PDList) > 0 {
			drivesResp, err := s.queryDrivesInfo()
			if err != nil {
				return nil, fmt.Errorf("failed to collect megaraid drive info: %s", err)
			}
			if err := s.collectMegaRaidDrives(mx, drivesResp); err != nil {
				return nil, err
			}
		}
	case driverNameSas:
		if err := s.collectMpt3sasControllersInfo(mx, cntrlResp); err != nil {
			return nil, fmt.Errorf("failed to collect mpt3sas controller info: %s", err)
		}
	default:
		return nil, fmt.Errorf("unknown driver: %s", driver)
	}

	return mx, nil
}
