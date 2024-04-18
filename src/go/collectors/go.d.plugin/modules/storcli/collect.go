// SPDX-License-Identifier: GPL-3.0-or-later

package storcli

import "fmt"

func (s *StorCli) collect() (map[string]int64, error) {
	cntrlResp, err := s.queryControllersInfo()
	if err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	if err := s.collectControllersInfo(mx, cntrlResp); err != nil {
		return nil, fmt.Errorf("error collecting controller info: %s", err)
	}

	drives := cntrlResp.Controllers[0].ResponseData.PDList
	driver := cntrlResp.Controllers[0].ResponseData.Version.DriverName
	if driver == "megaraid_sas" && len(drives) > 0 {
		drivesResp, err := s.queryDrivesInfo()
		if err != nil {
			return nil, fmt.Errorf("error collecting drives info: %s", err)
		}
		if err := s.collectMegaRaidDrives(mx, drivesResp); err != nil {
			return nil, err
		}
	}

	return mx, nil
}
