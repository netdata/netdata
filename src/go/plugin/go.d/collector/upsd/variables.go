// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

const varPrecision = 100

// https://networkupstools.org/docs/developer-guide.chunked/apas02.html
const (
	varBatteryCharge         = "battery.charge"
	varBatteryRuntime        = "battery.runtime"
	varBatteryVoltage        = "battery.voltage"
	varBatteryVoltageNominal = "battery.voltage.nominal"
	varBatteryType           = "battery.type"

	varInputVoltage          = "input.voltage"
	varInputVoltageNominal   = "input.voltage.nominal"
	varInputCurrent          = "input.current"
	varInputCurrentNominal   = "input.current.nominal"
	varInputFrequency        = "input.frequency"
	varInputFrequencyNominal = "input.frequency.nominal"

	varOutputVoltage          = "output.voltage"
	varOutputVoltageNominal   = "output.voltage.nominal"
	varOutputCurrent          = "output.current"
	varOutputCurrentNominal   = "output.current.nominal"
	varOutputFrequency        = "output.frequency"
	varOutputFrequencyNominal = "output.frequency.nominal"

	varUpsLoad             = "ups.load"
	varUpsRealPower        = "ups.realpower"
	varUpsRealPowerNominal = "ups.realpower.nominal"
	varUpsTemperature      = "ups.temperature"
	varUpsStatus           = "ups.status"

	varDeviceModel  = "device.model"
	varDeviceSerial = "device.serial"
	varDeviceMfr    = "device.mfr"
	varDeviceType   = "device.type"
)
