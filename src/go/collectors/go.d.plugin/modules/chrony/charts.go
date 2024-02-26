// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

var charts = module.Charts{
	{
		ID:    "stratum",
		Title: "Distance to the reference clock",
		Units: "level",
		Fam:   "stratum",
		Ctx:   "chrony.stratum",
		Dims: module.Dims{
			{ID: "stratum", Name: "stratum"},
		},
	},
	{
		ID:    "current_correction",
		Title: "Current correction",
		Units: "seconds",
		Fam:   "correction",
		Ctx:   "chrony.current_correction",
		Dims: module.Dims{
			{ID: "current_correction", Div: scaleFactor},
		},
	},
	{
		ID:    "root_delay",
		Title: "Network path delay to stratum-1",
		Units: "seconds",
		Fam:   "root",
		Ctx:   "chrony.root_delay",
		Dims: module.Dims{
			{ID: "root_delay", Div: scaleFactor},
		},
	},
	{
		ID:    "root_dispersion",
		Title: "Dispersion accumulated back to stratum-1",
		Units: "seconds",
		Fam:   "root",
		Ctx:   "chrony.root_dispersion",
		Dims: module.Dims{
			{ID: "root_dispersion", Div: scaleFactor},
		},
	},
	{
		ID:    "last_offset",
		Title: "Offset on the last clock update",
		Units: "seconds",
		Fam:   "offset",
		Ctx:   "chrony.last_offset",
		Dims: module.Dims{
			{ID: "last_offset", Name: "offset", Div: scaleFactor},
		},
	},
	{
		ID:    "rms_offset",
		Title: "Long-term average of the offset value",
		Units: "seconds",
		Fam:   "offset",
		Ctx:   "chrony.rms_offset",
		Dims: module.Dims{
			{ID: "rms_offset", Name: "offset", Div: scaleFactor},
		},
	},
	{
		ID:    "frequency",
		Title: "Frequency",
		Units: "ppm",
		Fam:   "frequency",
		Ctx:   "chrony.frequency",
		Dims: module.Dims{
			{ID: "frequency", Div: scaleFactor},
		},
	},
	{
		ID:    "residual_frequency",
		Title: "Residual frequency",
		Units: "ppm",
		Fam:   "frequency",
		Ctx:   "chrony.residual_frequency",
		Dims: module.Dims{
			{ID: "residual_frequency", Div: scaleFactor},
		},
	},
	{
		ID:    "skew",
		Title: "Skew",
		Units: "ppm",
		Fam:   "frequency",
		Ctx:   "chrony.skew",
		Dims: module.Dims{
			{ID: "skew", Div: scaleFactor},
		},
	},
	{
		ID:    "update_interval",
		Title: "Interval between the last two clock updates",
		Units: "seconds",
		Fam:   "updates",
		Ctx:   "chrony.update_interval",
		Dims: module.Dims{
			{ID: "update_interval", Div: scaleFactor},
		},
	},
	{
		ID:    "ref_measurement_time",
		Title: "Time since the last measurement",
		Units: "seconds",
		Fam:   "updates",
		Ctx:   "chrony.ref_measurement_time",
		Dims: module.Dims{
			{ID: "ref_measurement_time"},
		},
	},
	{
		ID:    "leap_status",
		Title: "Leap status",
		Units: "status",
		Fam:   "leap status",
		Ctx:   "chrony.leap_status",
		Dims: module.Dims{
			{ID: "leap_status_normal", Name: "normal"},
			{ID: "leap_status_insert_second", Name: "insert_second"},
			{ID: "leap_status_delete_second", Name: "delete_second"},
			{ID: "leap_status_unsynchronised", Name: "unsynchronised"},
		},
	},
	{
		ID:    "activity",
		Title: "Peers activity",
		Units: "sources",
		Fam:   "activity",
		Ctx:   "chrony.activity",
		Type:  module.Stacked,
		Dims: module.Dims{
			{ID: "online_sources", Name: "online"},
			{ID: "offline_sources", Name: "offline"},
			{ID: "burst_online_sources", Name: "burst_online"},
			{ID: "burst_offline_sources", Name: "burst_offline"},
			{ID: "unresolved_sources", Name: "unresolved"},
		},
	},
}
