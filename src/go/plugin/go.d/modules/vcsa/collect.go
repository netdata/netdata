// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import (
	"sync"
)

var componentHealthStatuses = []string{"green", "red", "yellow", "orange", "gray"}
var softwareHealthStatuses = []string{"green", "red", "orange", "gray"}

type vcsaHealthStatus struct {
	System           *string
	ApplMgmt         *string
	Load             *string
	Mem              *string
	Swap             *string
	DatabaseStorage  *string
	Storage          *string
	SoftwarePackages *string
}

func (vc *VCSA) collect() (map[string]int64, error) {
	err := vc.client.Ping()
	if err != nil {
		return nil, err
	}

	var status vcsaHealthStatus
	vc.scrapeHealth(&status)

	mx := make(map[string]int64)

	writeStatus(mx, "system", componentHealthStatuses, status.System)
	writeStatus(mx, "applmgmt", componentHealthStatuses, status.ApplMgmt)
	writeStatus(mx, "load", componentHealthStatuses, status.Load)
	writeStatus(mx, "mem", componentHealthStatuses, status.Mem)
	writeStatus(mx, "swap", componentHealthStatuses, status.Swap)
	writeStatus(mx, "database_storage", componentHealthStatuses, status.DatabaseStorage)
	writeStatus(mx, "storage", componentHealthStatuses, status.Storage)
	writeStatus(mx, "software_packages", softwareHealthStatuses, status.SoftwarePackages)

	return mx, nil
}

func (vc *VCSA) scrapeHealth(status *vcsaHealthStatus) {
	wg := &sync.WaitGroup{}

	scrape := func(fn func() (string, error), value **string) {
		v, err := fn()
		if err != nil {
			vc.Error(err)
			return
		}
		*value = &v
	}

	for _, fn := range []func(){
		func() { scrape(vc.client.System, &status.System) },
		func() { scrape(vc.client.ApplMgmt, &status.ApplMgmt) },
		func() { scrape(vc.client.Load, &status.Load) },
		func() { scrape(vc.client.DatabaseStorage, &status.DatabaseStorage) },
		func() { scrape(vc.client.Storage, &status.Storage) },
		func() { scrape(vc.client.Mem, &status.Mem) },
		func() { scrape(vc.client.Swap, &status.Swap) },
		func() { scrape(vc.client.SoftwarePackages, &status.SoftwarePackages) },
	} {
		fn := fn

		wg.Add(1)
		go func() { defer wg.Done(); fn() }()
	}

	wg.Wait()
}

func writeStatus(mx map[string]int64, key string, statuses []string, status *string) {
	if status == nil {
		return
	}

	var found bool
	for _, s := range statuses {
		mx[key+"_status_"+s] = boolToInt(s == *status)
		found = found || s == *status
	}
	mx[key+"_status_unknown"] = boolToInt(!found)
}

func boolToInt(v bool) int64 {
	if v {
		return 1
	}
	return 0
}
