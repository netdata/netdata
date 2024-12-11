// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

var (
	vcsaHealthCharts = module.Charts{
		systemHealthStatus.Copy(),
		applMgmtHealthChart.Copy(),
		loadHealthChart.Copy(),
		memHealthChart.Copy(),
		swapHealthChart.Copy(),
		dbStorageHealthChart.Copy(),
		storageHealthChart.Copy(),
		softwarePackagesHealthChart.Copy(),
	}

	systemHealthStatus = module.Chart{
		ID:    "system_health_status",
		Title: "VCSA Overall System health status",
		Units: "status",
		Fam:   "system",
		Ctx:   "vcsa.system_health_status",
		Dims: module.Dims{
			{ID: "system_status_green", Name: "green"},
			{ID: "system_status_red", Name: "red"},
			{ID: "system_status_yellow", Name: "yellow"},
			{ID: "system_status_orange", Name: "orange"},
			{ID: "system_status_gray", Name: "gray"},
			{ID: "system_status_unknown", Name: "unknown"},
		},
	}
	applMgmtHealthChart = module.Chart{
		ID:    "applmgmt_health_status",
		Title: "VCSA Appliance Management Service (applmgmt) health status",
		Units: "status",
		Fam:   "appliance mgmt service",
		Ctx:   "vcsa.applmgmt_health_status",
		Dims: module.Dims{
			{ID: "applmgmt_status_green", Name: "green"},
			{ID: "applmgmt_status_red", Name: "red"},
			{ID: "applmgmt_status_yellow", Name: "yellow"},
			{ID: "applmgmt_status_orange", Name: "orange"},
			{ID: "applmgmt_status_gray", Name: "gray"},
			{ID: "applmgmt_status_unknown", Name: "unknown"},
		},
	}
	loadHealthChart = module.Chart{
		ID:    "load_health_status",
		Title: "VCSA Load health status",
		Units: "status",
		Fam:   "load",
		Ctx:   "vcsa.load_health_status",
		Dims: module.Dims{
			{ID: "load_status_green", Name: "green"},
			{ID: "load_status_red", Name: "red"},
			{ID: "load_status_yellow", Name: "yellow"},
			{ID: "load_status_orange", Name: "orange"},
			{ID: "load_status_gray", Name: "gray"},
			{ID: "load_status_unknown", Name: "unknown"},
		},
	}
	memHealthChart = module.Chart{
		ID:    "mem_health_status",
		Title: "VCSA Memory health status",
		Units: "status",
		Fam:   "mem",
		Ctx:   "vcsa.mem_health_status",
		Dims: module.Dims{
			{ID: "mem_status_green", Name: "green"},
			{ID: "mem_status_red", Name: "red"},
			{ID: "mem_status_yellow", Name: "yellow"},
			{ID: "mem_status_orange", Name: "orange"},
			{ID: "mem_status_gray", Name: "gray"},
			{ID: "mem_status_unknown", Name: "unknown"},
		},
	}
	swapHealthChart = module.Chart{
		ID:    "swap_health_status",
		Title: "VCSA Swap health status",
		Units: "status",
		Fam:   "swap",
		Ctx:   "vcsa.swap_health_status",
		Dims: module.Dims{
			{ID: "swap_status_green", Name: "green"},
			{ID: "swap_status_red", Name: "red"},
			{ID: "swap_status_yellow", Name: "yellow"},
			{ID: "swap_status_orange", Name: "orange"},
			{ID: "swap_status_gray", Name: "gray"},
			{ID: "swap_status_unknown", Name: "unknown"},
		},
	}
	dbStorageHealthChart = module.Chart{
		ID:    "database_storage_health_status",
		Title: "VCSA Database Storage health status",
		Units: "status",
		Fam:   "db storage",
		Ctx:   "vcsa.database_storage_health_status",
		Dims: module.Dims{
			{ID: "database_storage_status_green", Name: "green"},
			{ID: "database_storage_status_red", Name: "red"},
			{ID: "database_storage_status_yellow", Name: "yellow"},
			{ID: "database_storage_status_orange", Name: "orange"},
			{ID: "database_storage_status_gray", Name: "gray"},
			{ID: "database_storage_status_unknown", Name: "unknown"},
		},
	}
	storageHealthChart = module.Chart{
		ID:    "storage_health_status",
		Title: "VCSA Storage health status",
		Units: "status",
		Fam:   "storage",
		Ctx:   "vcsa.storage_health_status",
		Dims: module.Dims{
			{ID: "storage_status_green", Name: "green"},
			{ID: "storage_status_red", Name: "red"},
			{ID: "storage_status_yellow", Name: "yellow"},
			{ID: "storage_status_orange", Name: "orange"},
			{ID: "storage_status_gray", Name: "gray"},
			{ID: "storage_status_unknown", Name: "unknown"},
		},
	}
	softwarePackagesHealthChart = module.Chart{
		ID:    "software_packages_health_status",
		Title: "VCSA Software Updates health status",
		Units: "status",
		Fam:   "software packages",
		Ctx:   "vcsa.software_packages_health_status",
		Dims: module.Dims{
			{ID: "software_packages_status_green", Name: "green"},
			{ID: "software_packages_status_red", Name: "red"},
			{ID: "software_packages_status_orange", Name: "orange"},
			{ID: "software_packages_status_gray", Name: "gray"},
			{ID: "software_packages_status_unknown", Name: "unknown"},
		},
	}
)
