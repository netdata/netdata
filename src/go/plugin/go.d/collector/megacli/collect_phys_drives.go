// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package megacli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"strings"
)

type (
	megaAdapter struct {
		number     string
		name       string
		state      string
		physDrives map[string]*megaPhysDrive
	}
	megaPhysDrive struct {
		adapterNumber          string
		number                 string
		wwn                    string
		slotNumber             string
		drivePosition          string
		pdType                 string
		mediaErrorCount        string
		predictiveFailureCount string
	}
)

var adapterStates = []string{
	"optimal",
	"degraded",
	"partially_degraded",
	"failed",
}

func (c *Collector) collectPhysDrives(mx map[string]int64) error {
	bs, err := c.exec.physDrivesInfo()
	if err != nil {
		return err
	}

	adapters, err := parsePhysDrivesInfo(bs)
	if err != nil {
		return err
	}
	if len(adapters) == 0 {
		return errors.New("no adapters found")
	}

	var drives int

	for _, ad := range adapters {
		if !c.adapters[ad.number] {
			c.adapters[ad.number] = true
			c.addAdapterCharts(ad)
		}

		px := fmt.Sprintf("adapter_%s_health_state_", ad.number)
		for _, st := range adapterStates {
			mx[px+st] = 0
		}
		st := strings.ReplaceAll(strings.ToLower(ad.state), " ", "_")
		mx[px+st] = 1

		for _, pd := range ad.physDrives {
			if !c.adapters[pd.wwn] {
				c.adapters[pd.wwn] = true
				c.addPhysDriveCharts(pd)
			}
			drives++

			px := fmt.Sprintf("phys_drive_%s_", pd.wwn)

			writeInt(mx, px+"media_error_count", pd.mediaErrorCount)
			writeInt(mx, px+"predictive_failure_count", pd.predictiveFailureCount)
		}
	}

	c.Debugf("found %d adapters, %d physical drives", len(c.adapters), drives)

	return nil
}

func parsePhysDrivesInfo(bs []byte) (map[string]*megaAdapter, error) {
	adapters := make(map[string]*megaAdapter)

	var ad *megaAdapter
	var pd *megaPhysDrive

	sc := bufio.NewScanner(bytes.NewReader(bs))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		switch {
		case strings.HasPrefix(line, "Adapter #"):
			idx := strings.TrimPrefix(line, "Adapter #")
			ad = &megaAdapter{number: idx, physDrives: make(map[string]*megaPhysDrive)}
			adapters[idx] = ad
		case strings.HasPrefix(line, "Name") && ad != nil:
			ad.name = getColonSepValue(line)
		case strings.HasPrefix(line, "State") && ad != nil:
			ad.state = getColonSepValue(line)
		case strings.HasPrefix(line, "PD:") && ad != nil:
			if parts := strings.Fields(line); len(parts) == 3 {
				idx := parts[1]
				pd = &megaPhysDrive{number: idx, adapterNumber: ad.number}
				ad.physDrives[idx] = pd
			}
		case strings.HasPrefix(line, "Slot Number:") && pd != nil:
			pd.slotNumber = getColonSepValue(line)
		case strings.HasPrefix(line, "Drive's position:") && pd != nil:
			pd.drivePosition = getColonSepValue(line)
		case strings.HasPrefix(line, "WWN:") && pd != nil:
			pd.wwn = getColonSepValue(line)
		case strings.HasPrefix(line, "PD Type:") && pd != nil:
			pd.pdType = getColonSepValue(line)
		case strings.HasPrefix(line, "Media Error Count:") && pd != nil:
			pd.mediaErrorCount = getColonSepNumValue(line)
		case strings.HasPrefix(line, "Predictive Failure Count:") && pd != nil:
			pd.predictiveFailureCount = getColonSepNumValue(line)
		}
	}

	return adapters, nil
}
