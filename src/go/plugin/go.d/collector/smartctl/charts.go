// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly

package smartctl

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioDeviceSmartStatus = module.Priority + iota
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

var deviceChartsTmpl = module.Charts{
	devicePowerOnTimeChartTmpl.Copy(),
	deviceTemperatureChartTmpl.Copy(),
	devicePowerCycleCountChartTmpl.Copy(),
	deviceSmartStatusChartTmpl.Copy(),
	deviceAtaSmartErrorLogCountChartTmpl.Copy(),
}

var (
	deviceSmartStatusChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_smart_status",
		Title:    "Device smart status",
		Units:    "status",
		Fam:      "smart status",
		Ctx:      "smartctl.device_smart_status",
		Type:     module.Line,
		Priority: prioDeviceSmartStatus,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_smart_status_passed", Name: "passed"},
			{ID: "device_%s_type_%s_smart_status_failed", Name: "failed"},
		},
	}
	deviceAtaSmartErrorLogCountChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_ata_smart_error_log_count",
		Title:    "Device ATA smart error log count",
		Units:    "logs",
		Fam:      "smart error log",
		Ctx:      "smartctl.device_ata_smart_error_log_count",
		Type:     module.Line,
		Priority: prioDeviceAtaSmartErrorLogCount,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_ata_smart_error_log_summary_count", Name: "error_log"},
		},
	}
	devicePowerOnTimeChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_power_on_time",
		Title:    "Device power on time",
		Units:    "seconds",
		Fam:      "power on time",
		Ctx:      "smartctl.device_power_on_time",
		Type:     module.Line,
		Priority: prioDevicePowerOnTime,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_power_on_time", Name: "power_on_time"},
		},
	}
	deviceTemperatureChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_temperature",
		Title:    "Device temperature",
		Units:    "Celsius",
		Fam:      "temperature",
		Ctx:      "smartctl.device_temperature",
		Type:     module.Line,
		Priority: prioDeviceTemperature,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_temperature", Name: "temperature"},
		},
	}
	devicePowerCycleCountChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_power_cycle_count",
		Title:    "Device power cycles",
		Units:    "cycles",
		Fam:      "power cycles",
		Ctx:      "smartctl.device_power_cycles_count",
		Type:     module.Line,
		Priority: prioDevicePowerCycleCount,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_power_cycle_count", Name: "power"},
		},
	}
)

var deviceScsiErrorLogChartsTmpl = module.Charts{
	deviceScsiReadErrorsChartTmpl.Copy(),
	deviceScsiWriteErrorsChartTmpl.Copy(),
	deviceScsiVerifyErrorsChartTmpl.Copy(),
}

var (
	deviceScsiReadErrorsChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_read_errors_rate",
		Title:    "Device read errors",
		Units:    "errors/s",
		Fam:      "scsi errors",
		Ctx:      "smartctl.device_read_errors_rate",
		Type:     module.Line,
		Priority: prioDeviceScsiReadErrors,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_scsi_error_log_read_total_errors_corrected", Name: "corrected", Algo: module.Incremental},
			{ID: "device_%s_type_%s_scsi_error_log_read_total_uncorrected_errors", Name: "uncorrected", Algo: module.Incremental},
		},
	}
	deviceScsiWriteErrorsChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_write_errors_rate",
		Title:    "Device write errors",
		Units:    "errors/s",
		Fam:      "scsi errors",
		Ctx:      "smartctl.device_write_errors_rate",
		Type:     module.Line,
		Priority: prioDeviceScsiWriteErrors,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_scsi_error_log_write_total_errors_corrected", Name: "corrected", Algo: module.Incremental},
			{ID: "device_%s_type_%s_scsi_error_log_read_total_uncorrected_errors", Name: "uncorrected", Algo: module.Incremental},
		},
	}
	deviceScsiVerifyErrorsChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_verify_errors_rate",
		Title:    "Device verify errors",
		Units:    "errors/s",
		Fam:      "scsi errors",
		Ctx:      "smartctl.device_verify_errors_rate",
		Type:     module.Line,
		Priority: prioDeviceScsiVerifyErrors,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_scsi_error_log_verify_total_errors_corrected", Name: "corrected", Algo: module.Incremental},
			{ID: "device_%s_type_%s_scsi_error_log_verify_total_uncorrected_errors", Name: "uncorrected", Algo: module.Incremental},
		},
	}
)

var (
	deviceSmartAttributeDecodedChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_smart_attr_%s",
		Title:    "Device smart attribute %s",
		Units:    "value",
		Fam:      "attr %s",
		Ctx:      "smartctl.device_smart_attr_%s",
		Type:     module.Line,
		Priority: prioDeviceSmartAttributeDecoded,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_attr_%s_decoded", Name: "%s"},
		},
	}
	deviceSmartAttributeNormalizedChartTmpl = module.Chart{
		ID:       "device_%s_type_%s_smart_attr_%s_normalized",
		Title:    "Device smart attribute normalized %s",
		Units:    "value",
		Fam:      "attr %s",
		Ctx:      "smartctl.device_smart_attr_%s_normalized",
		Type:     module.Line,
		Priority: prioDeviceSmartAttributeNormalized,
		Dims: module.Dims{
			{ID: "device_%s_type_%s_attr_%s_normalized", Name: "%s"},
		},
	}
)

func (c *Collector) addDeviceCharts(dev *smartDevice) {
	charts := module.Charts{}

	if cs := c.newDeviceCharts(dev); cs != nil && len(*cs) > 0 {
		if err := charts.Add(*cs...); err != nil {
			c.Warning(err)
		}
	}
	if cs := c.newDeviceSmartAttrCharts(dev); cs != nil && len(*cs) > 0 {
		if err := charts.Add(*cs...); err != nil {
			c.Warning(err)
		}
	}
	if cs := c.newDeviceScsiErrorLogCharts(dev); cs != nil && len(*cs) > 0 {
		if err := charts.Add(*cs...); err != nil {
			c.Warning(err)
		}
	}

	if err := c.Charts().Add(charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removeDeviceCharts(scanDev *scanDevice) {
	px := fmt.Sprintf("device_%s_%s_", scanDev.shortName(), scanDev.typ)

	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, px) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (c *Collector) newDeviceCharts(dev *smartDevice) *module.Charts {

	charts := deviceChartsTmpl.Copy()

	if _, ok := dev.powerOnTime(); !ok {
		_ = charts.Remove(devicePowerOnTimeChartTmpl.ID)
	}
	if _, ok := dev.temperature(); !ok {
		_ = charts.Remove(deviceTemperatureChartTmpl.ID)
	}
	if _, ok := dev.powerCycleCount(); !ok {
		_ = charts.Remove(devicePowerOnTimeChartTmpl.ID)
	}
	if _, ok := dev.smartStatusPassed(); !ok {
		_ = charts.Remove(deviceSmartStatusChartTmpl.ID)
	}
	if _, ok := dev.ataSmartErrorLogCount(); !ok {
		_ = charts.Remove(deviceAtaSmartErrorLogCountChartTmpl.ID)
	}

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, dev.deviceName(), dev.deviceType())
		chart.Labels = []module.Label{
			{Key: "device_name", Value: dev.deviceName()},
			{Key: "device_type", Value: dev.deviceType()},
			{Key: "model_name", Value: dev.modelName()},
			{Key: "serial_number", Value: dev.serialNumber()},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, dev.deviceName(), dev.deviceType())
		}
	}

	return charts
}

func (c *Collector) newDeviceSmartAttrCharts(dev *smartDevice) *module.Charts {
	attrs, ok := dev.ataSmartAttributeTable()
	if !ok {
		return nil
	}
	charts := module.Charts{}

	for _, attr := range attrs {
		if !isSmartAttrValid(attr) ||
			strings.HasPrefix(attr.name(), "Unknown") ||
			strings.HasPrefix(attr.name(), "Not_In_Use") {
			continue
		}

		cs := module.Charts{
			deviceSmartAttributeDecodedChartTmpl.Copy(),
			deviceSmartAttributeNormalizedChartTmpl.Copy(),
		}

		attrName := attributeNameMap(attr.name())
		cleanAttrName := cleanAttributeName(attrName)

		for _, chart := range cs {
			if chart.ID == deviceSmartAttributeDecodedChartTmpl.ID {
				chart.Units = attributeUnit(attrName)
			}
			chart.ID = fmt.Sprintf(chart.ID, dev.deviceName(), dev.deviceType(), cleanAttrName)
			chart.Title = fmt.Sprintf(chart.Title, attrName)
			chart.Fam = fmt.Sprintf(chart.Fam, cleanAttrName)
			chart.Ctx = fmt.Sprintf(chart.Ctx, cleanAttrName)
			chart.Labels = []module.Label{
				{Key: "device_name", Value: dev.deviceName()},
				{Key: "device_type", Value: dev.deviceType()},
				{Key: "model_name", Value: dev.modelName()},
				{Key: "serial_number", Value: dev.serialNumber()},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, dev.deviceName(), dev.deviceType(), cleanAttrName)
				dim.Name = fmt.Sprintf(dim.Name, cleanAttrName)
			}
		}

		if err := charts.Add(cs...); err != nil {
			c.Warning(err)
		}
	}

	return &charts
}

func (c *Collector) newDeviceScsiErrorLogCharts(dev *smartDevice) *module.Charts {
	if dev.deviceType() != "scsi" || !dev.data.Get("scsi_error_counter_log").Exists() {
		return nil
	}

	charts := deviceScsiErrorLogChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, dev.deviceName(), dev.deviceType())
		chart.Labels = []module.Label{
			{Key: "device_name", Value: dev.deviceName()},
			{Key: "device_type", Value: dev.deviceType()},
			{Key: "model_name", Value: dev.modelName()},
			{Key: "serial_number", Value: dev.serialNumber()},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, dev.deviceName(), dev.deviceType())
		}
	}

	return charts
}

var attrNameReplacer = strings.NewReplacer(" ", "_", "/", "_")

func cleanAttributeName(attrName string) string {
	return strings.ToLower(attrNameReplacer.Replace(attrName))
}

func attributeUnit(attrName string) string {
	units := map[string]string{
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

	if unit, ok := units[attrName]; ok {
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

func attributeNameMap(attrName string) string {
	// TODO: Handle Vendor-Specific S.M.A.R.T. Attribute Naming
	// S.M.A.R.T. attribute names can vary slightly between vendors (e.g., "Thermal_Throttle_St" vs. "Thermal_Throttle_Status").
	// This function ensures consistent naming.
	return attrName
}
