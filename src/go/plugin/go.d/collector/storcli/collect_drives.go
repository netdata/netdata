// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package storcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type drivesInfoResponse struct {
	Controllers []struct {
		CommandStatus struct {
			Controller int    `json:"Controller"`
			Status     string `json:"Status"`
		} `json:"Command Status"`
		ResponseData map[string]json.RawMessage `json:"Response Data"`
	} `json:"Controllers"`
}

type (
	driveInfo struct {
		EIDSlt string `json:"EID:Slt"`
		//DID    int    `json:"DID"`
		//State  string `json:"State"`
		//DG     int    `json:"DG"` // FIX: can be integer or "-"
		//Size   string `json:"Size"`
		//Intf   string `json:"Intf"`
		Med string `json:"Med"`
		//SED    string `json:"SED"`
		//PI     string `json:"PI"`
		//SeSz   string `json:"SeSz"`
		//Model  string `json:"Model"`
		//Sp     string `json:"Sp"`
		//Type   string `json:"Type"`
	}
	driveState struct {
		MediaErrorCount        storNumber `json:"Media Error Count"`
		OtherErrorCount        storNumber `json:"Other Error Count"`
		DriveTemperature       string     `json:"Drive Temperature"`
		PredictiveFailureCount storNumber `json:"Predictive Failure Count"`
		SmartAlertFlagged      string     `json:"S.M.A.R.T alert flagged by drive"`
	}
	driveAttrs struct {
		WWN         string `json:"WWN"`
		DeviceSpeed string `json:"Device Speed"`
		LinkSpeed   string `json:"Link Speed"`
	}
)

type storNumber string // some int values can be 'N/A'

func (n *storNumber) UnmarshalJSON(b []byte) error { *n = storNumber(b); return nil }

func (c *Collector) collectMegaRaidDrives(mx map[string]int64, resp *drivesInfoResponse) error {
	if resp == nil {
		return nil
	}

	for _, cntrl := range resp.Controllers {
		var ids []string
		for k := range cntrl.ResponseData {
			if !strings.HasSuffix(k, "Detailed Information") {
				continue
			}
			parts := strings.Fields(k) // Drive /c0/e252/s0 - Detailed Information
			if len(parts) < 2 {
				continue
			}
			id := parts[1]
			if strings.IndexByte(id, '/') == -1 {
				continue
			}
			ids = append(ids, id)
		}

		cntrlIdx := cntrl.CommandStatus.Controller

		for _, id := range ids {
			info, err := getDriveInfo(cntrl.ResponseData, id)
			if err != nil {
				return err
			}
			data, err := getDriveDetailedInfo(cntrl.ResponseData, id)
			if err != nil {
				return err
			}
			state, err := getDriveState(data, id)
			if err != nil {
				return err
			}
			attrs, err := getDriveAttrs(data, id)
			if err != nil {
				return err
			}

			if attrs.WWN == "" {
				continue
			}

			if !c.drives[attrs.WWN] {
				c.drives[attrs.WWN] = true
				c.addPhysDriveCharts(cntrlIdx, info, state, attrs)
			}

			px := fmt.Sprintf("phys_drive_%s_cntrl_%d_", attrs.WWN, cntrlIdx)

			if v, ok := parseInt(string(state.MediaErrorCount)); ok {
				mx[px+"media_error_count"] = v
			}
			if v, ok := parseInt(string(state.OtherErrorCount)); ok {
				mx[px+"other_error_count"] = v
			}
			if v, ok := parseInt(string(state.PredictiveFailureCount)); ok {
				mx[px+"predictive_failure_count"] = v
			}
			if v, ok := parseInt(getTemperature(state.DriveTemperature)); ok {
				mx[px+"temperature"] = v
			}
			for _, st := range []string{"active", "inactive"} {
				mx[px+"smart_alert_status_"+st] = 0
			}
			if state.SmartAlertFlagged == "Yes" {
				mx[px+"smart_alert_status_active"] = 1
			} else {
				mx[px+"smart_alert_status_inactive"] = 1
			}
		}
	}

	return nil
}

func (c *Collector) queryDrivesInfo() (*drivesInfoResponse, error) {
	bs, err := c.exec.drivesInfo()
	if err != nil {
		return nil, err
	}

	if len(bs) == 0 {
		return nil, errors.New("empty response")
	}

	var resp drivesInfoResponse
	if err := json.Unmarshal(bs, &resp); err != nil {
		return nil, err
	}

	if len(resp.Controllers) == 0 {
		return nil, errors.New("no controllers found")
	}
	if st := resp.Controllers[0].CommandStatus.Status; st != "Success" {
		return nil, fmt.Errorf("command status error: %s", st)
	}

	return &resp, nil
}

func getDriveInfo(respData map[string]json.RawMessage, id string) (*driveInfo, error) {
	k := fmt.Sprintf("Drive %s", id)
	raw, ok := respData[k]
	if !ok {
		return nil, fmt.Errorf("drive info not found for '%s'", id)
	}

	var drive []driveInfo
	if err := json.Unmarshal(raw, &drive); err != nil {
		return nil, err
	}

	if len(drive) == 0 {
		return nil, fmt.Errorf("drive info not found for '%s'", id)
	}

	return &drive[0], nil
}

func getDriveDetailedInfo(respData map[string]json.RawMessage, id string) (map[string]json.RawMessage, error) {
	k := fmt.Sprintf("Drive %s - Detailed Information", id)
	raw, ok := respData[k]
	if !ok {
		return nil, fmt.Errorf("drive detailed info not found for '%s'", id)
	}

	var info map[string]json.RawMessage
	if err := json.Unmarshal(raw, &info); err != nil {
		return nil, err
	}

	return info, nil
}

func getDriveState(driveDetailedInfo map[string]json.RawMessage, id string) (*driveState, error) {
	k := fmt.Sprintf("Drive %s State", id)
	raw, ok := driveDetailedInfo[k]
	if !ok {
		return nil, fmt.Errorf("drive detailed info state not found for '%s'", id)
	}

	var state driveState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

func getDriveAttrs(driveDetailedInfo map[string]json.RawMessage, id string) (*driveAttrs, error) {
	k := fmt.Sprintf("Drive %s Device attributes", id)
	raw, ok := driveDetailedInfo[k]
	if !ok {
		return nil, fmt.Errorf("drive detailed info state not found for '%s'", id)
	}

	var state driveAttrs
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}

	return &state, nil
}

func getTemperature(temp string) string {
	// ' 28C (82.40 F)' (drive) or '33C' (bbu)
	i := strings.IndexByte(temp, 'C')
	if i == -1 {
		return ""
	}
	return strings.TrimSpace(temp[:i])
}

func parseInt(s string) (int64, bool) {
	i, err := strconv.ParseInt(s, 10, 64)
	return i, err == nil
}
