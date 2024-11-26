// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import (
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
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

func (c *Collector) collect() (map[string]int64, error) {
	err := c.client.Ping()
	if err != nil {
		return nil, err
	}

	var status vcsaHealthStatus
	c.scrapeHealth(&status)

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

func (c *Collector) scrapeHealth(status *vcsaHealthStatus) {
	wg := &sync.WaitGroup{}

	scrape := func(fn func() (string, error), value **string) {
		v, err := fn()
		if err != nil {
			c.Error(err)
			return
		}
		*value = &v
	}

	for _, fn := range []func(){
		func() { scrape(c.client.System, &status.System) },
		func() { scrape(c.client.ApplMgmt, &status.ApplMgmt) },
		func() { scrape(c.client.Load, &status.Load) },
		func() { scrape(c.client.DatabaseStorage, &status.DatabaseStorage) },
		func() { scrape(c.client.Storage, &status.Storage) },
		func() { scrape(c.client.Mem, &status.Mem) },
		func() { scrape(c.client.Swap, &status.Swap) },
		func() { scrape(c.client.SoftwarePackages, &status.SoftwarePackages) },
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
		mx[key+"_status_"+s] = metrix.Bool(s == *status)
		found = found || s == *status
	}
	mx[key+"_status_unknown"] = metrix.Bool(!found)
}
