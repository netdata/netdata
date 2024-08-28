package lmsensors

import (
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TODO(mdlayher): why does scanning work if device file isn't a symlink,
// even though it is in the actual filesystem (and the actual filesystem
// exhibits the same behavior)?

func TestScannerScan(t *testing.T) {
	tests := []struct {
		name    string
		fs      filesystem
		devices []*Device
	}{
		{
			name: "power_meter device",
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0",
					"/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device": "../../../ACPI0000:00",
				},
				files: []memoryFile{
					{
						name: "/sys/class/hwmon",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/class/hwmon/hwmon0",
						dirEntry: &memoryDirEntry{
							mode: os.ModeSymlink,
						},
					},
					{
						name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/name",
						err:  os.ErrNotExist,
					},
					{
						name:     "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/hwmon/hwmon0/device",
						dirEntry: &memoryDirEntry{
							// mode: os.ModeSymlink,
						},
					},
					{
						name:     "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/name",
						contents: "power_meter",
					},
					{
						name:     "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_average",
						contents: "345000000",
					},
					{
						name:     "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_average_interval",
						contents: "1000",
					},
					{
						name:     "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_is_battery",
						contents: "0",
					},
					{
						name:     "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_model_number",
						contents: "Intel(R) Node Manager",
					},
					{
						name:     "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_oem_info",
						contents: "Meter measures total domain",
					},
					{
						name:     "/sys/devices/LNXSYSTM:00/device:00/ACPI0000:00/power1_serial_number",
						contents: "",
					},
				},
			},
			devices: []*Device{{
				Name: "power_meter-00",
				Sensors: []Sensor{
					&PowerSensor{
						Name:            "power1",
						Average:         345.0,
						AverageInterval: 1 * time.Second,
						Battery:         false,
						ModelNumber:     "Intel(R) Node Manager",
						OEMInfo:         "Meter measures total domain",
						SerialNumber:    "",
					},
				},
			}},
		},
		{
			name: "acpitz device",
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/virtual/hwmon/hwmon0",
				},
				files: []memoryFile{
					{
						name: "/sys/class/hwmon",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/class/hwmon/hwmon0",
						dirEntry: &memoryDirEntry{
							mode: os.ModeSymlink,
						},
					},
					{
						name: "/sys/devices/virtual/hwmon/hwmon0",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name:     "/sys/devices/virtual/hwmon/hwmon0/name",
						contents: "acpitz",
					},
					{
						name:     "/sys/devices/virtual/hwmon/hwmon0/temp1_crit",
						contents: "105000",
					},
					{
						name:     "/sys/devices/virtual/hwmon/hwmon0/temp1_input",
						contents: "27800",
					},
				},
			},
			devices: []*Device{{
				Name: "acpitz-00",
				Sensors: []Sensor{
					&TemperatureSensor{
						Name:          "temp1",
						Input:         27.8,
						Critical:      105.0,
						CriticalAlarm: false,
					},
				},
			}},
		},
		{
			name: "coretemp device",
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon1":                              "../../devices/platform/coretemp.0/hwmon/hwmon1",
					"/sys/devices/platform/coretemp.0/hwmon/hwmon1/device": "../../../coretemp.0",
				},
				files: []memoryFile{
					{
						name: "/sys/class/hwmon",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/class/hwmon/hwmon1",
						dirEntry: &memoryDirEntry{
							mode: os.ModeSymlink,
						},
					},
					{
						name: "/sys/devices/platform/coretemp.0",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/devices/platform/coretemp.0/hwmon/hwmon1/name",
						err:  os.ErrNotExist,
					},
					{
						name:     "/sys/devices/platform/coretemp.0/hwmon/hwmon1/device",
						dirEntry: &memoryDirEntry{
							// mode: os.ModeSymlink,
						},
					},
					{
						name:     "/sys/devices/platform/coretemp.0/name",
						contents: "coretemp",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_crit",
						contents: "100000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_crit_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_input",
						contents: "40000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_label",
						contents: "Core 0",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_max",
						contents: "80000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_crit",
						contents: "100000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_crit_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_input",
						contents: "42000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_label",
						contents: "Core 1",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_max",
						contents: "80000",
					},
				},
			},
			devices: []*Device{{
				Name: "coretemp-00",
				Sensors: []Sensor{
					&TemperatureSensor{
						Name:          "temp1",
						Label:         "Core 0",
						Input:         40.0,
						High:          80.0,
						Critical:      100.0,
						CriticalAlarm: false,
					},
					&TemperatureSensor{
						Name:          "temp2",
						Label:         "Core 1",
						Input:         42.0,
						High:          80.0,
						Critical:      100.0,
						CriticalAlarm: false,
					},
				},
			}},
		},
		{
			name: "it8728 device",
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon2":                             "../../devices/platform/it87.2608/hwmon/hwmon2",
					"/sys/devices/platform/it87.2608/hwmon/hwmon2/device": "../../../it87.2608",
				},
				files: []memoryFile{
					{
						name: "/sys/class/hwmon",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/class/hwmon/hwmon2",
						dirEntry: &memoryDirEntry{
							mode: os.ModeSymlink,
						},
					},
					{
						name: "/sys/devices/platform/it87.2608",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/devices/platform/it87.2608/hwmon/hwmon2/name",
						err:  os.ErrNotExist,
					},
					{
						name:     "/sys/devices/platform/it87.2608/hwmon/hwmon2/device",
						dirEntry: &memoryDirEntry{
							// mode: os.ModeSymlink,
						},
					},
					{
						name:     "/sys/devices/platform/it87.2608/name",
						contents: "it8728",
					},
					{
						name:     "/sys/devices/platform/it87.2608/fan1_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/it87.2608/fan1_beep",
						contents: "1",
					},
					{
						name:     "/sys/devices/platform/it87.2608/fan1_input",
						contents: "1010",
					},
					{
						name:     "/sys/devices/platform/it87.2608/fan1_min",
						contents: "10",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in0_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in0_beep",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in0_input",
						contents: "1056",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in0_max",
						contents: "3060",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in1_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in1_beep",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in1_input",
						contents: "3384",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in1_label",
						contents: "3VSB",
					},
					{
						name:     "/sys/devices/platform/it87.2608/in1_max",
						contents: "6120",
					},
					{
						name:     "/sys/devices/platform/it87.2608/intrusion0_alarm",
						contents: "1",
					},
					{
						name:     "/sys/devices/platform/it87.2608/temp1_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/it87.2608/temp1_beep",
						contents: "1",
					},
					{
						name:     "/sys/devices/platform/it87.2608/temp1_input",
						contents: "43000",
					},
					{
						name:     "/sys/devices/platform/it87.2608/temp1_max",
						contents: "127000",
					},
					{
						name:     "/sys/devices/platform/it87.2608/temp1_type",
						contents: "4",
					},
				},
			},
			devices: []*Device{{
				Name: "it8728-00",
				Sensors: []Sensor{
					&FanSensor{
						Name:    "fan1",
						Alarm:   false,
						Beep:    true,
						Input:   1010,
						Minimum: 10,
					},
					&VoltageSensor{
						Name:    "in0",
						Alarm:   false,
						Beep:    false,
						Input:   1.056,
						Maximum: 3.060,
					},
					&VoltageSensor{
						Name:    "in1",
						Label:   "3VSB",
						Alarm:   false,
						Beep:    false,
						Input:   3.384,
						Maximum: 6.120,
					},
					&IntrusionSensor{
						Name:  "intrusion0",
						Alarm: true,
					},
					&TemperatureSensor{
						Name:  "temp1",
						Alarm: false,
						Beep:  true,
						Type:  TemperatureSensorTypeThermistor,
						Input: 43.0,
						High:  127.0,
					},
				},
			}},
		},
		{
			name: "multiple coretemp devices",
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon1":                              "../../devices/platform/coretemp.0/hwmon/hwmon1",
					"/sys/class/hwmon/hwmon2":                              "../../devices/platform/coretemp.1/hwmon/hwmon2",
					"/sys/devices/platform/coretemp.0/hwmon/hwmon1/device": "../../../coretemp.0",
					"/sys/devices/platform/coretemp.1/hwmon/hwmon2/device": "../../../coretemp.1",
				},
				files: []memoryFile{
					{
						name: "/sys/class/hwmon",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/class/hwmon/hwmon1",
						dirEntry: &memoryDirEntry{
							mode: os.ModeSymlink,
						},
					},
					{
						name: "/sys/class/hwmon/hwmon2",
						dirEntry: &memoryDirEntry{
							mode: os.ModeSymlink,
						},
					},
					{
						name: "/sys/devices/platform/coretemp.0",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/devices/platform/coretemp.0/hwmon/hwmon1/name",
						err:  os.ErrNotExist,
					},
					{
						name:     "/sys/devices/platform/coretemp.0/hwmon/hwmon1/device",
						dirEntry: &memoryDirEntry{
							// mode: os.ModeSymlink,
						},
					},
					{
						name:     "/sys/devices/platform/coretemp.0/name",
						contents: "coretemp",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_crit",
						contents: "100000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_crit_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_input",
						contents: "40000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_label",
						contents: "Core 0",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp1_max",
						contents: "80000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_crit",
						contents: "100000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_crit_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_input",
						contents: "42000",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_label",
						contents: "Core 1",
					},
					{
						name:     "/sys/devices/platform/coretemp.0/temp2_max",
						contents: "80000",
					},
					{
						name: "/sys/devices/platform/coretemp.1",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/devices/platform/coretemp.1/hwmon/hwmon2/name",
						err:  os.ErrNotExist,
					},
					{
						name:     "/sys/devices/platform/coretemp.1/hwmon/hwmon2/device",
						dirEntry: &memoryDirEntry{
							// mode: os.ModeSymlink,
						},
					},
					{
						name:     "/sys/devices/platform/coretemp.1/name",
						contents: "coretemp",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp1_crit",
						contents: "100000",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp1_crit_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp1_input",
						contents: "38000",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp1_label",
						contents: "Core 0",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp1_max",
						contents: "80000",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp2_crit",
						contents: "100000",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp2_crit_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp2_input",
						contents: "37000",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp2_label",
						contents: "Core 1",
					},
					{
						name:     "/sys/devices/platform/coretemp.1/temp2_max",
						contents: "80000",
					},
				},
			},
			devices: []*Device{
				{
					Name: "coretemp-00",
					Sensors: []Sensor{
						&TemperatureSensor{
							Name:          "temp1",
							Label:         "Core 0",
							Input:         40.0,
							High:          80.0,
							Critical:      100.0,
							CriticalAlarm: false,
						},
						&TemperatureSensor{
							Name:          "temp2",
							Label:         "Core 1",
							Input:         42.0,
							High:          80.0,
							Critical:      100.0,
							CriticalAlarm: false,
						},
					},
				},
				{
					Name: "coretemp-01",
					Sensors: []Sensor{
						&TemperatureSensor{
							Name:          "temp1",
							Label:         "Core 0",
							Input:         38.0,
							High:          80.0,
							Critical:      100.0,
							CriticalAlarm: false,
						},
						&TemperatureSensor{
							Name:          "temp2",
							Label:         "Core 1",
							Input:         37.0,
							High:          80.0,
							Critical:      100.0,
							CriticalAlarm: false,
						},
					},
				},
			},
		},
		{
			name: "sfc device",
			fs: &memoryFilesystem{
				symlinks: map[string]string{
					"/sys/class/hwmon/hwmon0": "../../devices/pci0000:00/0000:00:02.0/0000:03:00.0/hwmon/hwmon0",
					"/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/hwmon/hwmon0/device": "../../../0000:03:00.0",
				},
				files: []memoryFile{
					{
						name: "/sys/class/hwmon",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/class/hwmon/hwmon0",
						dirEntry: &memoryDirEntry{
							mode: os.ModeSymlink,
						},
					},
					{
						name: "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0",
						dirEntry: &memoryDirEntry{
							isDir: true,
						},
					},
					{
						name: "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/hwmon/hwmon0/name",
						err:  os.ErrNotExist,
					},
					{
						name:     "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/hwmon/hwmon0/device",
						dirEntry: &memoryDirEntry{
							// mode: os.ModeSymlink,
						},
					},
					{
						name:     "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/name",
						contents: "sfc",
					},
					{
						name:     "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/curr1_alarm",
						contents: "0",
					},
					{
						name:     "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/curr1_crit",
						contents: "18000",
					},
					{
						name:     "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/curr1_input",
						contents: "7624",
					},
					{
						name:     "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/curr1_label",
						contents: "0.9V supply current",
					},
					{
						name:     "/sys/devices/pci0000:00/0000:00:02.0/0000:03:00.0/curr1_max",
						contents: "16000",
					},
				},
			},
			devices: []*Device{{
				Name: "sfc-00",
				Sensors: []Sensor{
					&CurrentSensor{
						Name:     "curr1",
						Label:    "0.9V supply current",
						Alarm:    false,
						Input:    7.624,
						Maximum:  16.0,
						Critical: 18.0,
					},
				},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Scanner{fs: tt.fs}

			devices, err := s.Scan()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if want, got := tt.devices, devices; !reflect.DeepEqual(want, got) {
				t.Fatalf("unexpected Devices:\n- want:\n%v\n-  got:\n%v",
					devicesStr(want), devicesStr(got))
			}
		})
	}
}

func devicesStr(ds []*Device) string {
	var out string
	for _, d := range ds {
		out += fmt.Sprintf("device: %q [%d sensors]\n", d.Name, len(d.Sensors))

		for _, s := range d.Sensors {
			out += fmt.Sprintf("  - sensor: %#v\n", s)
		}
	}

	return out
}

var _ filesystem = &memoryFilesystem{}

// A memoryFilesystem is an in-memory implementation of filesystem, used for
// tests.
type memoryFilesystem struct {
	symlinks map[string]string
	files    []memoryFile
}

func (fs *memoryFilesystem) ReadFile(filename string) (string, error) {
	for _, f := range fs.files {
		if f.name == filename {
			return f.contents, nil
		}
	}

	return "", fmt.Errorf("readfile: file %q not in memory", filename)
}

func (fs *memoryFilesystem) Readlink(name string) (string, error) {
	if l, ok := fs.symlinks[name]; ok {
		return l, nil
	}

	return "", fmt.Errorf("readlink: symlink %q not in memory", name)
}

func (fs *memoryFilesystem) Stat(name string) (os.FileInfo, error) {
	for _, f := range fs.files {
		if f.name == name {
			de := f.dirEntry
			if de == nil {
				de = &memoryDirEntry{}
			}
			info, _ := de.Info()
			return info, f.err
		}
	}

	return nil, fmt.Errorf("stat: file %q not in memory", name)
}

func (fs *memoryFilesystem) WalkDir(root string, walkFn fs.WalkDirFunc) error {
	if _, err := fs.Stat(root); err != nil {
		return err
	}

	for _, f := range fs.files {
		// Only walk paths under the specified root
		if !strings.HasPrefix(f.name, root) {
			continue
		}

		de := f.dirEntry
		if de == nil {
			de = &memoryDirEntry{}
		}

		if err := walkFn(f.name, de, nil); err != nil {
			return err
		}
	}

	return nil
}

// A memoryFile is an in-memory file used by memoryFilesystem.
type memoryFile struct {
	name     string
	contents string
	dirEntry fs.DirEntry
	err      error
}

var _ fs.DirEntry = &memoryDirEntry{}

// A memoryDirEntry is a fs.DirEntry used by memoryFiles.
type memoryDirEntry struct {
	name  string
	mode  os.FileMode
	isDir bool
}

func (fi *memoryDirEntry) Name() string               { return fi.name }
func (fi *memoryDirEntry) Type() os.FileMode          { return fi.mode }
func (fi *memoryDirEntry) IsDir() bool                { return fi.isDir }
func (fi *memoryDirEntry) Info() (fs.FileInfo, error) { return fi, nil }
func (fi *memoryDirEntry) Sys() interface{}           { return nil }
func (fi *memoryDirEntry) Size() int64                { return 0 }
func (fi *memoryDirEntry) Mode() os.FileMode          { return fi.Type() }
func (fi *memoryDirEntry) ModTime() time.Time         { return time.Now() }
