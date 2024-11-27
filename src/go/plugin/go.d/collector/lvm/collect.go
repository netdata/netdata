// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || netbsd

package lvm

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type lvsReport struct {
	Report []struct {
		Lv []struct {
			VGName          string `json:"vg_name"`
			LVName          string `json:"lv_name"`
			LVSize          string `json:"lv_size"`
			DataPercent     string `json:"data_percent"`
			MetadataPercent string `json:"metadata_percent"`
			LVAttr          string `json:"lv_attr"`
		} `json:"lv"`
	} `json:"report"`
}

func (c *Collector) collect() (map[string]int64, error) {
	bs, err := c.exec.lvsReportJson()
	if err != nil {
		return nil, err
	}

	var report lvsReport
	if err = json.Unmarshal(bs, &report); err != nil {
		return nil, err
	}

	mx := make(map[string]int64)

	for _, r := range report.Report {
		for _, lv := range r.Lv {
			if lv.VGName == "" || lv.LVName == "" {
				continue
			}

			if !isThinPool(lv.LVAttr) {
				c.Debugf("skipping lv '%s' vg '%s': not a thin pool", lv.LVName, lv.VGName)
				continue
			}

			key := fmt.Sprintf("lv_%s_vg_%s", lv.LVName, lv.VGName)
			if !c.lvmThinPools[key] {
				c.addLVMThinPoolCharts(lv.LVName, lv.VGName)
				c.lvmThinPools[key] = true
			}
			if v, ok := parseFloat(lv.DataPercent); ok {
				mx[key+"_data_percent"] = int64(v * 100)
			}
			if v, ok := parseFloat(lv.MetadataPercent); ok {
				mx[key+"_metadata_percent"] = int64(v * 100)
			}
		}
	}

	return mx, nil
}

func isThinPool(lvAttr string) bool {
	return getLVType(lvAttr) == "thin_pool"
}

func getLVType(lvAttr string) string {
	if len(lvAttr) == 0 {
		return ""
	}

	// https://man7.org/linux/man-pages/man8/lvs.8.html#NOTES
	switch lvAttr[0] {
	case 'C':
		return "cache"
	case 'm':
		return "mirrored"
	case 'M':
		return "mirrored_without_initial_sync"
	case 'o':
		return "origin"
	case 'O':
		return "origin_with_merging_snapshot"
	case 'g':
		return "integrity"
	case 'r':
		return "raid"
	case 'R':
		return "raid_without_initial_sync"
	case 's':
		return "snapshot"
	case 'S':
		return "merging_snapshot"
	case 'p':
		return "pvmove"
	case 'v':
		return "virtual"
	case 'i':
		return "mirror_or_raid_image"
	case 'I':
		return "mirror_or_raid_mage_out_of_sync"
	case 'l':
		return "log_device"
	case 'c':
		return "under_conversion"
	case 'V':
		return "thin_volume"
	case 't':
		return "thin_pool"
	case 'T':
		return "thin_pool_data"
	case 'd':
		return "vdo_pool"
	case 'D':
		return "vdo_pool_data"
	case 'e':
		return "raid_or_pool_metadata"
	default:
		return ""
	}
}

func parseFloat(s string) (float64, bool) {
	if s == "-" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	return v, err == nil
}
