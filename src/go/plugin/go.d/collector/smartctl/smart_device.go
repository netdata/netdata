// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package smartctl

import (
	"strings"

	"github.com/tidwall/gjson"
)

func newSmartDevice(deviceData *gjson.Result) *smartDevice {
	return &smartDevice{
		data: deviceData,
	}
}

type smartDevice struct {
	data *gjson.Result
}

func (d *smartDevice) deviceName() string {
	v := d.data.Get("device.name").String()
	return strings.TrimPrefix(v, "/dev/")
}

func (d *smartDevice) deviceType() string {
	return d.data.Get("device.type").String()
}

func (d *smartDevice) serialNumber() string {
	return d.data.Get("serial_number").String()
}

func (d *smartDevice) modelName() string {
	for _, s := range []string{"model_name", "scsi_model_name"} {
		if v := d.data.Get(s); v.Exists() {
			return v.String()
		}
	}
	return "unknown"
}

func (d *smartDevice) powerOnTime() (int64, bool) {
	h := d.data.Get("power_on_time.hours")
	if !h.Exists() {
		return 0, false
	}
	m := d.data.Get("power_on_time.minutes")
	return h.Int()*60*60 + m.Int()*60, true
}

func (d *smartDevice) temperature() (int64, bool) {
	v := d.data.Get("temperature.current")
	return v.Int(), v.Exists()
}

func (d *smartDevice) powerCycleCount() (int64, bool) {
	for _, s := range []string{"power_cycle_count", "scsi_start_stop_cycle_counter.accumulated_start_stop_cycles"} {
		if v := d.data.Get(s); v.Exists() {
			return v.Int(), true
		}
	}
	return 0, false
}

func (d *smartDevice) smartStatusPassed() (bool, bool) {
	v := d.data.Get("smart_status.passed")
	return v.Bool(), v.Exists()
}

func (d *smartDevice) ataSmartErrorLogCount() (int64, bool) {
	v := d.data.Get("ata_smart_error_log.summary.count")
	return v.Int(), v.Exists()
}

func (d *smartDevice) ataSmartAttributeTable() ([]*smartAttribute, bool) {
	table := d.data.Get("ata_smart_attributes.table")
	if !table.Exists() || !table.IsArray() {
		return nil, false
	}

	var attrs []*smartAttribute

	for _, data := range table.Array() {
		attrs = append(attrs, newSmartDeviceAttribute(data))
	}

	return attrs, true
}

func newSmartDeviceAttribute(attrData gjson.Result) *smartAttribute {
	return &smartAttribute{
		data: attrData,
	}
}

type smartAttribute struct {
	data gjson.Result
}

func (a *smartAttribute) id() string {
	return a.data.Get("id").String()
}

func (a *smartAttribute) name() string {
	return a.data.Get("name").String()
}

func (a *smartAttribute) value() string {
	return a.data.Get("value").String()
}

func (a *smartAttribute) rawValue() string {
	return a.data.Get("raw.value").String()
}

func (a *smartAttribute) rawString() string {
	return a.data.Get("raw.string").String()
}
