// SPDX-License-Identifier: GPL-3.0-or-later

package smartctl

import (
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

const (
	prioDeviceSmartStatus = collectorapi.Priority + iota
	prioDeviceAtaSmartErrorLogCount
	prioDevicePowerOnTime
	prioDeviceTemperature
	prioDevicePowerCycleCount

	prioDeviceScsiReadErrors
	prioDeviceScsiWriteErrors
	prioDeviceScsiVerifyErrors

	prioDeviceSmartAttributeDecoded
	prioDeviceSmartAttributeNormalized
)

var deviceChartsTmpl = collectorapi.Charts{
	devicePowerOnTimeChartTmpl.Copy(),
	deviceTemperatureChartTmpl.Copy(),
	devicePowerCycleCountChartTmpl.Copy(),
	deviceSmartStatusChartTmpl.Copy(),
	deviceAtaSmartErrorLogCountChartTmpl.Copy(),
}

var (
	deviceSmartStatusChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_smart_status",
		Title:    "Device smart status",
		Units:    "status",
		Fam:      "smart status",
		Ctx:      "smartctl.device_smart_status",
		Type:     collectorapi.Line,
		Priority: prioDeviceSmartStatus,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_smart_status_passed", Name: "passed"},
			{ID: "device_%s_type_%s_smart_status_failed", Name: "failed"},
		},
	}
	deviceAtaSmartErrorLogCountChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_ata_smart_error_log_count",
		Title:    "Device ATA smart error log count",
		Units:    "logs",
		Fam:      "smart error log",
		Ctx:      "smartctl.device_ata_smart_error_log_count",
		Type:     collectorapi.Line,
		Priority: prioDeviceAtaSmartErrorLogCount,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_ata_smart_error_log_summary_count", Name: "error_log"},
		},
	}
	devicePowerOnTimeChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_power_on_time",
		Title:    "Device power on time",
		Units:    "seconds",
		Fam:      "power on time",
		Ctx:      "smartctl.device_power_on_time",
		Type:     collectorapi.Line,
		Priority: prioDevicePowerOnTime,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_power_on_time", Name: "power_on_time"},
		},
	}
	deviceTemperatureChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_temperature",
		Title:    "Device temperature",
		Units:    "Celsius",
		Fam:      "temperature",
		Ctx:      "smartctl.device_temperature",
		Type:     collectorapi.Line,
		Priority: prioDeviceTemperature,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_temperature", Name: "temperature"},
		},
	}
	devicePowerCycleCountChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_power_cycle_count",
		Title:    "Device power cycles",
		Units:    "cycles",
		Fam:      "power cycles",
		Ctx:      "smartctl.device_power_cycles_count",
		Type:     collectorapi.Line,
		Priority: prioDevicePowerCycleCount,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_power_cycle_count", Name: "power"},
		},
	}
)

var deviceScsiErrorLogChartsTmpl = collectorapi.Charts{
	deviceScsiReadErrorsChartTmpl.Copy(),
	deviceScsiWriteErrorsChartTmpl.Copy(),
	deviceScsiVerifyErrorsChartTmpl.Copy(),
}

var (
	deviceScsiReadErrorsChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_read_errors_rate",
		Title:    "Device read errors",
		Units:    "errors/s",
		Fam:      "scsi errors",
		Ctx:      "smartctl.device_read_errors_rate",
		Type:     collectorapi.Line,
		Priority: prioDeviceScsiReadErrors,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_scsi_error_log_read_total_errors_corrected", Name: "corrected", Algo: collectorapi.Incremental},
			{ID: "device_%s_type_%s_scsi_error_log_read_total_uncorrected_errors", Name: "uncorrected", Algo: collectorapi.Incremental},
		},
	}
	deviceScsiWriteErrorsChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_write_errors_rate",
		Title:    "Device write errors",
		Units:    "errors/s",
		Fam:      "scsi errors",
		Ctx:      "smartctl.device_write_errors_rate",
		Type:     collectorapi.Line,
		Priority: prioDeviceScsiWriteErrors,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_scsi_error_log_write_total_errors_corrected", Name: "corrected", Algo: collectorapi.Incremental},
			{ID: "device_%s_type_%s_scsi_error_log_write_total_uncorrected_errors", Name: "uncorrected", Algo: collectorapi.Incremental},
		},
	}
	deviceScsiVerifyErrorsChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_verify_errors_rate",
		Title:    "Device verify errors",
		Units:    "errors/s",
		Fam:      "scsi errors",
		Ctx:      "smartctl.device_verify_errors_rate",
		Type:     collectorapi.Line,
		Priority: prioDeviceScsiVerifyErrors,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_scsi_error_log_verify_total_errors_corrected", Name: "corrected", Algo: collectorapi.Incremental},
			{ID: "device_%s_type_%s_scsi_error_log_verify_total_uncorrected_errors", Name: "uncorrected", Algo: collectorapi.Incremental},
		},
	}
)

var (
	deviceSmartAttributeDecodedChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_smart_attr_%s",
		Title:    "Device smart attribute %s",
		Units:    "value",
		Fam:      "attr %s",
		Ctx:      "smartctl.device_smart_attr_%s",
		Type:     collectorapi.Line,
		Priority: prioDeviceSmartAttributeDecoded,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_attr_%s_decoded", Name: "%s"},
		},
	}
	deviceSmartAttributeNormalizedChartTmpl = collectorapi.Chart{
		ID:       "device_%s_type_%s_smart_attr_%s_normalized",
		Title:    "Device smart attribute normalized %s",
		Units:    "value",
		Fam:      "attr %s",
		Ctx:      "smartctl.device_smart_attr_%s_normalized",
		Type:     collectorapi.Line,
		Priority: prioDeviceSmartAttributeNormalized,
		Dims: collectorapi.Dims{
			{ID: "device_%s_type_%s_attr_%s_normalized", Name: "%s"},
		},
	}
)

func buildDeviceCharts(dev *smartDevice, id deviceIdentity, smartAttrs smartAttributeIdentities) (collectorapi.Charts, error) {
	var charts collectorapi.Charts
	for _, group := range []*collectorapi.Charts{
		newDeviceCharts(dev, id),
		newDeviceSmartAttrCharts(dev, id, smartAttrs),
		newDeviceScsiErrorLogCharts(dev, id),
	} {
		if group != nil {
			charts = append(charts, (*group)...)
		}
	}

	validated := collectorapi.Charts{}
	if err := validated.Add(charts...); err != nil {
		return nil, err
	}
	return validated, nil
}

func (c *Collector) addDeviceCharts(charts collectorapi.Charts) error {
	candidate := append(collectorapi.Charts(nil), (*c.Charts())...)
	if err := candidate.Add(charts...); err != nil {
		return err
	}
	*c.Charts() = candidate
	return nil
}

func (c *Collector) reconcileDeviceCharts(current, desired collectorapi.Charts) (collectorapi.Charts, error) {
	if sameDeviceCharts(current, desired) {
		return current, nil
	}

	owned := make(map[*collectorapi.Chart]bool, len(current))
	for _, chart := range current {
		owned[chart] = true
	}
	candidate := make(collectorapi.Charts, 0, len(*c.Charts())-len(current)+len(desired))
	for _, chart := range *c.Charts() {
		if !owned[chart] {
			candidate = append(candidate, chart)
		}
	}
	if err := candidate.Add(desired...); err != nil {
		return nil, err
	}

	currentByID := make(map[string]*collectorapi.Chart, len(current))
	for _, chart := range current {
		currentByID[chart.ID] = chart
	}

	active := make(collectorapi.Charts, 0, len(desired))
	var additions collectorapi.Charts
	for _, chart := range desired {
		if existing, ok := currentByID[chart.ID]; ok {
			// Preserve the chart object when its public ID survives the attachment refresh.
			if !sameChartSpec(existing, chart) {
				updateChartSpec(existing, chart)
			}
			active = append(active, existing)
			delete(currentByID, chart.ID)
		} else {
			active = append(active, chart)
			additions = append(additions, chart)
		}
	}

	for _, chart := range currentByID {
		chart.MarkRemove()
		chart.MarkNotCreated()
	}
	if err := c.Charts().Add(additions...); err != nil {
		return nil, err
	}
	return active, nil
}

func sameDeviceCharts(left, right collectorapi.Charts) bool {
	if len(left) != len(right) {
		return false
	}
	byID := make(map[string]*collectorapi.Chart, len(left))
	for _, chart := range left {
		byID[chart.ID] = chart
	}
	for _, chart := range right {
		if existing, ok := byID[chart.ID]; !ok || !sameChartSpec(existing, chart) {
			return false
		}
	}
	return true
}

func sameChartSpec(left, right *collectorapi.Chart) bool {
	return left.OverModule == right.OverModule &&
		left.IDSep == right.IDSep &&
		left.ID == right.ID &&
		left.OverID == right.OverID &&
		left.Title == right.Title &&
		left.Units == right.Units &&
		left.Fam == right.Fam &&
		left.Ctx == right.Ctx &&
		left.Type == right.Type &&
		left.Priority == right.Priority &&
		left.UpdateEvery == right.UpdateEvery &&
		left.SkipGaps == right.SkipGaps &&
		left.Opts == right.Opts &&
		slices.Equal(left.Labels, right.Labels) &&
		sameDims(left.Dims, right.Dims) &&
		sameVars(left.Vars, right.Vars)
}

func sameDims(left, right collectorapi.Dims) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if *left[i] != *right[i] {
			return false
		}
	}
	return true
}

func sameVars(left, right collectorapi.Vars) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if *left[i] != *right[i] {
			return false
		}
	}
	return true
}

func updateChartSpec(dst, src *collectorapi.Chart) {
	dst.OverModule = src.OverModule
	dst.IDSep = src.IDSep
	dst.ID = src.ID
	dst.OverID = src.OverID
	dst.Title = src.Title
	dst.Units = src.Units
	dst.Fam = src.Fam
	dst.Ctx = src.Ctx
	dst.Type = src.Type
	dst.Priority = src.Priority
	dst.UpdateEvery = src.UpdateEvery
	dst.SkipGaps = src.SkipGaps
	dst.Opts = src.Opts
	dst.Labels = src.Labels
	dst.Dims = src.Dims
	dst.Vars = src.Vars
	dst.MarkNotCreated()
}

func removeDeviceCharts(charts collectorapi.Charts) {
	for _, chart := range charts {
		chart.MarkRemove()
		chart.MarkNotCreated()
	}
}

func newDeviceCharts(dev *smartDevice, id deviceIdentity) *collectorapi.Charts {
	charts := deviceChartsTmpl.Copy()

	if _, ok := dev.powerOnTime(); !ok {
		_ = charts.Remove(devicePowerOnTimeChartTmpl.ID)
	}
	if _, ok := dev.temperature(); !ok {
		_ = charts.Remove(deviceTemperatureChartTmpl.ID)
	}
	if _, ok := dev.powerCycleCount(); !ok {
		_ = charts.Remove(devicePowerCycleCountChartTmpl.ID)
	}
	if _, ok := dev.smartStatusPassed(); !ok {
		_ = charts.Remove(deviceSmartStatusChartTmpl.ID)
	}
	if _, ok := dev.ataSmartErrorLogCount(); !ok {
		_ = charts.Remove(deviceAtaSmartErrorLogCountChartTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, id.name, id.typ)
		chart.Labels = deviceChartLabels(dev)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, id.name, id.typ)
		}
	}

	return charts
}

func newDeviceSmartAttrCharts(dev *smartDevice, id deviceIdentity, smartAttrs smartAttributeIdentities) *collectorapi.Charts {
	attrs, ok := dev.ataSmartAttributeTable()
	if !ok {
		return nil
	}
	charts := collectorapi.Charts{}

	for _, attr := range attrs {
		if !isSmartAttrChartable(attr) {
			continue
		}

		cs := collectorapi.Charts{
			deviceSmartAttributeDecodedChartTmpl.Copy(),
			deviceSmartAttributeNormalizedChartTmpl.Copy(),
		}

		attrName := attr.name()
		identity := smartAttrs.resolve(attr)
		cleanAttrName := identity.name

		for _, chart := range cs {
			if chart.ID == deviceSmartAttributeDecodedChartTmpl.ID {
				chart.Units = attributeUnit(attrName)
			}
			chart.ID = fmt.Sprintf(chart.ID, id.name, id.typ, cleanAttrName)
			chart.Title = fmt.Sprintf(chart.Title, attrName)
			chart.Fam = fmt.Sprintf(chart.Fam, cleanAttrName)
			chart.Ctx = fmt.Sprintf(chart.Ctx, cleanAttrName)
			chart.Labels = deviceChartLabels(dev)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, id.name, id.typ, cleanAttrName)
				dim.Name = fmt.Sprintf(dim.Name, cleanAttrName)
			}
		}

		charts = append(charts, cs...)
	}

	return &charts
}

func newDeviceScsiErrorLogCharts(dev *smartDevice, id deviceIdentity) *collectorapi.Charts {
	if dev.deviceType() != "scsi" || !dev.data.Get("scsi_error_counter_log").Exists() {
		return nil
	}

	charts := deviceScsiErrorLogChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, id.name, id.typ)
		chart.Labels = deviceChartLabels(dev)
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, id.name, id.typ)
		}
	}

	return charts
}

func (c *Collector) warnSmartAttributeCollisions(dev *smartDevice, smartAttrs smartAttributeIdentities) {
	warned := make(map[string]bool)
	for _, identity := range smartAttrs {
		if identity.name == identity.baseName || warned[identity.baseName] {
			continue
		}
		c.Warningf("device '%s' type '%s': SMART attributes normalize to '%s'; using attribute IDs to disambiguate", dev.deviceName(), dev.deviceType(), identity.baseName)
		warned[identity.baseName] = true
	}
}

func deviceChartLabels(dev *smartDevice) []collectorapi.Label {
	return []collectorapi.Label{
		{Key: "device_name", Value: dev.deviceName()},
		{Key: "device_type", Value: dev.deviceType()},
		{Key: "model_name", Value: dev.modelName()},
		{Key: "serial_number", Value: dev.serialNumber()},
	}
}

var attrNameReplacer = strings.NewReplacer("/", "_")

func cleanAttributeName(attrName string) string {
	attrName, _ = replaceIDWhitespace(attrNameReplacer.Replace(attrName))
	return strings.ToLower(attrName)
}

var smartAttributeUnits = map[string]string{
	"Airflow_Temperature_Cel": "Celsius",
	"Case_Temperature":        "Celsius",
	"Drive_Temperature":       "Celsius",
	"Temperature_Case":        "Celsius",
	"Temperature_Celsius":     "Celsius",
	"Temperature_Internal":    "Celsius",
	"Power_On_Hours":          "hours",
	"Spin_Up_Time":            "milliseconds",
	"Media_Wearout_Indicator": "percent",
	"Percent_Life_Remaining":  "percent",
	"Percent_Lifetime_Remain": "percent",
	"Total_LBAs_Read":         "sectors",
	"Total_LBAs_Written":      "sectors",
	"Offline_Uncorrectable":   "sectors",
	"Pending_Sector_Count":    "sectors",
	"Reallocated_Sector_Ct":   "sectors",
	"Current_Pending_Sector":  "sectors",
	"Reported_Uncorrect":      "errors",
	"Command_Timeout":         "events",
}

func attributeUnit(attrName string) string {
	if unit, ok := smartAttributeUnits[attrName]; ok {
		return unit
	}

	// TODO: convert to bytes during data collection? (examples: NAND_Writes_32MiB, Flash_Writes_GiB)
	if strings.HasSuffix(attrName, "MiB") || strings.HasSuffix(attrName, "GiB") {
		if strings.Contains(attrName, "Writes") {
			return "writes"
		}
		if strings.Contains(attrName, "Reads") {
			return "reads"
		}
	}

	if strings.Contains(attrName, "Error") {
		return "errors"
	}

	for _, s := range []string{"_Count", "_Cnt", "_Ct"} {
		if strings.HasSuffix(attrName, s) {
			return "events"
		}
	}

	return "value"
}
