package lmsensors

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestScanner_Scan(t *testing.T) {
	var tests = map[string]struct {
		fs          filesystem
		wantDevices []*Chip
	}{
		"Power sensor": {
			wantDevices: []*Chip{
				{
					Name:       "power_meter",
					UniqueName: "power_meter-acpi-b37a4ed3",
					SysDevice:  "LNXSYSTM:00/device:00/ACPI0000:00",
					Sensors: Sensors{
						Power: []*PowerSensor{
							{
								Name:           "power1",
								Label:          "some_label",
								Alarm:          ptr(false),
								Average:        ptr(345.0),
								AverageHighest: ptr(345.0),
								AverageLowest:  ptr(345.0),
								AverageMin:     ptr(345.0),
								Input:          ptr(345.0),
								InputHighest:   ptr(345.0),
								InputLowest:    ptr(345.0),
								Accuracy:       ptr(34.5),
								Cap:            ptr(345.0),
								CapMax:         ptr(345.0),
								CapMin:         ptr(345.0),
								Max:            ptr(345.0),
								CritMax:        ptr(345.0),
							},
						},
					}},
			},
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0",
					"/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device": "../../../ACPI0000:00",
				},
				files: []memoryFile{
					{name: "/sys/class/hwmon", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/class/hwmon/hwmon0", de: &memoryDirEntry{mode: os.ModeSymlink}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/name", err: os.ErrNotExist},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device", de: &memoryDirEntry{}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/name", val: "power_meter"},

					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_label", val: "some_label"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_alarm", val: "0"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_average", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_average_highest", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_average_lowest", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_average_max", val: "unknown"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_average_min", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_input", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_input_highest", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_input_lowest", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_accuracy", val: "34.5%"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_cap", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_cap_max", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_cap_min", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_max", val: "345000000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_crit", val: "345000000"},
				},
			},
		},
		"Temperature sensor": {
			wantDevices: []*Chip{
				{
					Name:       "temp_meter",
					UniqueName: "temp_meter-pci-e3b89088",
					SysDevice:  "pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0",
					Sensors: Sensors{
						Temperature: []*TemperatureSensor{
							{
								Name:        "temp1",
								Label:       "some_label",
								Alarm:       ptr(false),
								TempTypeRaw: 1,
								Input:       ptr(42.0),
								Max:         ptr(42.0),
								Min:         ptr(42.0),
								CritMin:     ptr(42.0),
								CritMax:     ptr(42.0),
								Emergency:   ptr(42.0),
								Lowest:      ptr(42.0),
								Highest:     ptr(42.0),
							},
						},
					}},
			},
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0",
					"/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/device": "../../../0000:81:00.0",
				},
				files: []memoryFile{
					{name: "/sys/class/hwmon", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/class/hwmon/hwmon0", de: &memoryDirEntry{mode: os.ModeSymlink}},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/hwmon/hwmon0/name", err: os.ErrNotExist},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/hwmon/hwmon0/device", de: &memoryDirEntry{}},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/name", val: "temp_meter"},

					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_label", val: "some_label"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_alarm", val: "0"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_type", val: "1"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_max", val: "42000"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_min", val: "42000"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_input", val: "42000"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_crit", val: "42000"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_emergency", val: "42000"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_lcrit", val: "42000"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_lowest", val: "42000"},
					{name: "/sys/devices/pci0000:80/0000:80:01.4/0000:81:00.0/nvme/nvme0/hwmon0/temp1_highest", val: "42000"},
				},
			},
		},
		"Voltage sensor": {
			wantDevices: []*Chip{
				{
					Name:       "voltage_meter",
					UniqueName: "voltage_meter-acpi-b37a4ed3",
					SysDevice:  "LNXSYSTM:00/device:00/ACPI0000:00",
					Sensors: Sensors{
						Voltage: []*VoltageSensor{
							{
								Name:    "in1",
								Label:   "some_label",
								Alarm:   ptr(false),
								Input:   ptr(42.0),
								Average: ptr(42.0),
								Min:     ptr(42.0),
								Max:     ptr(42.0),
								CritMin: ptr(42.0),
								CritMax: ptr(42.0),
								Lowest:  ptr(42.0),
								Highest: ptr(42.0),
							},
						},
					}},
			},
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0",
					"/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device": "../../../ACPI0000:00",
				},
				files: []memoryFile{
					{name: "/sys/class/hwmon", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/class/hwmon/hwmon0", de: &memoryDirEntry{mode: os.ModeSymlink}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/name", err: os.ErrNotExist},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device", de: &memoryDirEntry{}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/name", val: "voltage_meter"},

					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_label", val: "some_label"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_alarm", val: "0"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_input", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_average", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_min", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_max", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_lcrit", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_crit", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_lowest", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/in1_highest", val: "42000"},
				},
			},
		},
		"Fan sensor": {
			wantDevices: []*Chip{
				{
					Name:       "fan_meter",
					UniqueName: "fan_meter-acpi-b37a4ed3",
					SysDevice:  "LNXSYSTM:00/device:00/ACPI0000:00",
					Sensors: Sensors{
						Fan: []*FanSensor{
							{
								Name:   "fan1",
								Label:  "some_label",
								Alarm:  ptr(false),
								Input:  ptr(42.0),
								Min:    ptr(42.0),
								Max:    ptr(42.0),
								Target: ptr(42.0),
							},
						},
					}},
			},
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0",
					"/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device": "../../../ACPI0000:00",
				},
				files: []memoryFile{
					{name: "/sys/class/hwmon", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/class/hwmon/hwmon0", de: &memoryDirEntry{mode: os.ModeSymlink}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/name", err: os.ErrNotExist},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device", de: &memoryDirEntry{}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/name", val: "fan_meter"},

					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/fan1_label", val: "some_label"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/fan1_alarm", val: "0"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/fan1_input", val: "42"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/fan1_min", val: "42"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/fan1_max", val: "42"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/fan1_target", val: "42"},
				},
			},
		},
		"Energy sensor": {
			wantDevices: []*Chip{
				{
					Name:       "energy_meter",
					UniqueName: "energy_meter-acpi-b37a4ed3",
					SysDevice:  "LNXSYSTM:00/device:00/ACPI0000:00",
					Sensors: Sensors{
						Energy: []*EnergySensor{
							{
								Name:  "energy1",
								Label: "some_label",
								Input: ptr(42.0),
							},
						},
					}},
			},
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0",
					"/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device": "../../../ACPI0000:00",
				},
				files: []memoryFile{
					{name: "/sys/class/hwmon", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/class/hwmon/hwmon0", de: &memoryDirEntry{mode: os.ModeSymlink}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/name", err: os.ErrNotExist},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device", de: &memoryDirEntry{}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/name", val: "energy_meter"},

					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/energy1_label", val: "some_label"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/energy1_alarm", val: "0"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/energy1_input", val: "42000000"},
				},
			},
		},
		"Current sensor": {
			wantDevices: []*Chip{
				{
					Name:       "current_meter",
					UniqueName: "current_meter-acpi-b37a4ed3",
					SysDevice:  "LNXSYSTM:00/device:00/ACPI0000:00",
					Sensors: Sensors{
						Current: []*CurrentSensor{
							{
								Name:    "curr1",
								Label:   "some_label",
								Alarm:   ptr(false),
								Max:     ptr(42.0),
								Min:     ptr(42.0),
								CritMin: ptr(42.0),
								CritMax: ptr(42.0),
								Input:   ptr(42.0),
								Average: ptr(42.0),
								Lowest:  ptr(42.0),
								Highest: ptr(42.0),
							},
						},
					}},
			},
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0",
					"/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device": "../../../ACPI0000:00",
				},
				files: []memoryFile{
					{name: "/sys/class/hwmon", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/class/hwmon/hwmon0", de: &memoryDirEntry{mode: os.ModeSymlink}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/name", err: os.ErrNotExist},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device", de: &memoryDirEntry{}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/name", val: "current_meter"},

					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_label", val: "some_label"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_alarm", val: "0"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_max", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_min", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_lcrit", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_crit", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_input", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_average", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_lowest", val: "42000"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/curr1_highest", val: "42000"},
				},
			},
		},
		"Humidity sensor": {
			wantDevices: []*Chip{
				{
					Name:       "humidity_meter",
					UniqueName: "humidity_meter-acpi-b37a4ed3",
					SysDevice:  "LNXSYSTM:00/device:00/ACPI0000:00",
					Sensors: Sensors{
						Humidity: []*HumiditySensor{
							{
								Name:  "humidity1",
								Label: "some_label",
								Input: ptr(42.0),
							},
						},
					}},
			},
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0",
					"/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device": "../../../ACPI0000:00",
				},
				files: []memoryFile{
					{name: "/sys/class/hwmon", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/class/hwmon/hwmon0", de: &memoryDirEntry{mode: os.ModeSymlink}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/name", err: os.ErrNotExist},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device", de: &memoryDirEntry{}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/name", val: "humidity_meter"},

					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/humidity1_label", val: "some_label"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/humidity1_alarm", val: "0"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/humidity1_input", val: "42000"},
				},
			},
		},
		"Intrusion sensor": {
			wantDevices: []*Chip{
				{
					Name:       "intrusion_meter",
					UniqueName: "intrusion_meter-acpi-b37a4ed3",
					SysDevice:  "LNXSYSTM:00/device:00/ACPI0000:00",
					Sensors: Sensors{
						Intrusion: []*IntrusionSensor{
							{
								Name:  "intrusion1",
								Label: "some_label",
								Alarm: ptr(false),
							},
						},
					}},
			},
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0",
					"/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device": "../../../ACPI0000:00",
				},
				files: []memoryFile{
					{name: "/sys/class/hwmon", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/class/hwmon/hwmon0", de: &memoryDirEntry{mode: os.ModeSymlink}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00", de: &memoryDirEntry{isDir: true}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/name", err: os.ErrNotExist},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device", de: &memoryDirEntry{}},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/name", val: "intrusion_meter"},

					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/intrusion1_label", val: "some_label"},
					{name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/intrusion1_alarm", val: "0"},
				},
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sc := New()
			sc.fs = test.fs

			devices, err := sc.Scan()
			require.NoError(t, err)

			for _, dev := range devices {
				for _, sn := range dev.Sensors.Voltage {
					require.NotZerof(t, sn.ReadTime, "zero read time: [voltage] dev '%s' sensor '%s'", dev.Name, sn.Name)
					sn.ReadTime = 0
				}
				for _, sn := range dev.Sensors.Fan {
					require.NotZerof(t, sn.ReadTime, "zero read time: [fan] dev '%s' sensor '%s'", dev.Name, sn.Name)
					sn.ReadTime = 0
				}
				for _, sn := range dev.Sensors.Temperature {
					require.NotZerof(t, sn.ReadTime, "zero read time: [temp] dev '%s' sensor '%s'", dev.Name, sn.Name)
					sn.ReadTime = 0
				}
				for _, sn := range dev.Sensors.Current {
					require.NotZerof(t, sn.ReadTime, "zero read time: [curr] dev '%s' sensor '%s'", dev.Name, sn.Name)
					sn.ReadTime = 0
				}
				for _, sn := range dev.Sensors.Power {
					require.NotZerof(t, sn.ReadTime, "zero read time: [power] dev '%s' sensor '%s'", dev.Name, sn.Name)
					sn.ReadTime = 0
				}
				for _, sn := range dev.Sensors.Energy {
					require.NotZerof(t, sn.ReadTime, "zero read time: [energy] dev '%s' sensor '%s'", dev.Name, sn.Name)
					sn.ReadTime = 0
				}
				for _, sn := range dev.Sensors.Humidity {
					require.NotZerof(t, sn.ReadTime, "zero read time: [humidity] dev '%s' sensor '%s'", dev.Name, sn.Name)
					sn.ReadTime = 0
				}
				for _, sn := range dev.Sensors.Intrusion {
					require.NotZerof(t, sn.ReadTime, "zero read time: [intrusion] dev '%s' sensor '%s'", dev.Name, sn.Name)
					sn.ReadTime = 0
				}
			}

			require.Equal(t, test.wantDevices, devices)
		})
	}
}

type memoryFilesystem struct {
	symlinks map[string]string
	files    []memoryFile
}

func (m *memoryFilesystem) ReadDir(name string) ([]fs.DirEntry, error) {
	if !slices.ContainsFunc(m.files, func(file memoryFile) bool { return file.name == name }) {
		return nil, fmt.Errorf("readdir: dir %s not in memory", name)
	}
	var des []fs.DirEntry
	for _, v := range m.files {
		if strings.HasPrefix(v.name, name) {
			des = append(des, &memoryDirEntry{name: filepath.Base(v.name), isDir: false})
		}
	}
	return des, nil
}

func (m *memoryFilesystem) ReadFile(filename string) (string, error) {
	for _, f := range m.files {
		if f.name == filename {
			return f.val, nil
		}
	}

	return "", fmt.Errorf("readfile: file %q not in memory", filename)
}

func (m *memoryFilesystem) Readlink(name string) (string, error) {
	if l, ok := m.symlinks[name]; ok {
		return l, nil
	}

	return "", fmt.Errorf("readlink: symlink %q not in memory", name)
}

func (m *memoryFilesystem) Stat(name string) (os.FileInfo, error) {
	for _, f := range m.files {
		if f.name == name {
			de := f.de
			if de == nil {
				de = &memoryDirEntry{}
			}
			info, _ := de.Info()
			return info, f.err
		}
	}

	return nil, fmt.Errorf("stat: file %q not in memory", name)
}

func (m *memoryFilesystem) WalkDir(root string, walkFn fs.WalkDirFunc) error {
	if _, err := m.Stat(root); err != nil {
		return err
	}

	for _, f := range m.files {
		if !strings.HasPrefix(f.name, root) {
			continue
		}

		de := f.de
		if de == nil {
			de = &memoryDirEntry{}
		}

		if err := walkFn(f.name, de, nil); err != nil {
			return err
		}
	}

	return nil
}

type memoryFile struct {
	name string
	val  string
	de   fs.DirEntry
	err  error
}

type memoryDirEntry struct {
	name  string
	mode  os.FileMode
	isDir bool
}

func (fi *memoryDirEntry) Name() string               { return fi.name }
func (fi *memoryDirEntry) Type() os.FileMode          { return fi.mode }
func (fi *memoryDirEntry) IsDir() bool                { return fi.isDir }
func (fi *memoryDirEntry) Info() (fs.FileInfo, error) { return fi, nil }
func (fi *memoryDirEntry) Sys() any                   { return nil }
func (fi *memoryDirEntry) Size() int64                { return 0 }
func (fi *memoryDirEntry) Mode() os.FileMode          { return fi.Type() }
func (fi *memoryDirEntry) ModTime() time.Time         { return time.Now() }
