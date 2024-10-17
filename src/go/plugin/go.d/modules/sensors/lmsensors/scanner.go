package lmsensors

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudflare/cfssl/scan/crypto/sha1"

	"github.com/netdata/netdata/go/plugins/logger"
)

// New creates a new Scanner.
func New() *Scanner {
	return &Scanner{
		fs: &systemFilesystem{},
	}
}

// A Scanner scans for Devices, so data can be read from their Sensors.
type Scanner struct {
	*logger.Logger

	fs filesystem
}

// Scan scans for Devices and their Sensors.
func (sc *Scanner) Scan() ([]*Chip, error) {
	paths, err := sc.detectDevicePaths()
	if err != nil {
		return nil, err
	}

	sc.Debugf("sysfs scanner: found %d paths", len(paths))

	var chips []*Chip

	for _, path := range paths {
		sc.Debugf("sysfs scanner: scanning %s", path)

		var chip Chip

		rawSns := make(rawSensors)

		des, err := sc.fs.ReadDir(path)
		if err != nil {
			return nil, err
		}

		for _, de := range des {
			if !de.Type().IsRegular() || shouldSkip(de.Name()) {
				continue
			}

			filePath := filepath.Join(path, de.Name())

			now := time.Now()
			content, err := sc.fs.ReadFile(filePath)
			if err != nil {
				sc.Debugf("sysfs scanner: failed to read '%s': %v", filePath, err)
				continue
			}
			since := time.Since(now)

			sc.Debugf("sysfs scanner: reading file '%s' took %s", filePath, since)

			if de.Name() == "name" {
				chip.Name = content
				continue
			}

			// Sensor names in format "sensor#_foo", e.g. "temp1_input"
			feat, subfeat, ok := strings.Cut(de.Name(), "_")
			if !ok {
				continue
			}

			// power average_max can be unknown (https://github.com/netdata/netdata/issues/18805)
			if content == "unknown" && subfeat != "label" {
				continue
			}

			if _, ok := rawSns[feat]; !ok {
				rawSns[feat] = make(map[string]rawValue)
			}

			rawSns[feat][subfeat] = rawValue{value: content, readTime: since}
		}

		sensors, err := parseSensors(rawSns)
		if err != nil {
			return nil, fmt.Errorf("sysfs scanner: failed to parse (device '%s', path '%s'): %v", chip.Name, path, err)
		}

		if sensors != nil {
			chip.Sensors = *sensors
		}

		chip.SysDevice = getDevicePath(path)
		chip.UniqueName = fmt.Sprintf("%s-%s-%s", chip.Name, getBusType(chip.SysDevice), getHash(chip.SysDevice))

		chips = append(chips, &chip)
	}

	return chips, nil
}

// detectDevicePaths performs a filesystem walk to paths where devices may reside on Linux.
func (sc *Scanner) detectDevicePaths() ([]string, error) {
	const lookPath = "/sys/class/hwmon"

	var paths []string

	err := sc.fs.WalkDir(lookPath, func(path string, de os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if de.Type()&os.ModeSymlink == 0 {
			return nil
		}

		dest, err := sc.fs.Readlink(path)
		if err != nil {
			return err
		}

		dest = filepath.Join(lookPath, filepath.Clean(dest))

		// Symlink destination has a file called name, meaning a sensor exists here and data can be retrieved
		fi, err := sc.fs.Stat(filepath.Join(dest, "name"))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if err == nil && fi.Mode().IsRegular() {
			paths = append(paths, dest)
			return nil
		}

		// Symlink destination has another symlink called device, which can be read and used to retrieve data
		device := filepath.Join(dest, "device")
		fi, err = sc.fs.Stat(device)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		}

		if fi.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		device, err = sc.fs.Readlink(device)
		if err != nil {
			return err
		}

		dest = filepath.Join(dest, filepath.Clean(device))

		// Symlink destination has a file called name, meaning a sensor exists here and data can be retrieved
		if _, err := sc.fs.Stat(filepath.Join(dest, "name")); err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			return nil
		}

		paths = append(paths, dest)

		return nil
	})

	return paths, err
}

func getDevicePath(path string) string {
	devPath, err := filepath.EvalSymlinks(filepath.Join(path, "device"))
	if err != nil {
		devPath = path
		if i := strings.Index(devPath, "/hwmon"); i > 0 {
			devPath = devPath[:i]
		}
	}
	return strings.TrimPrefix(devPath, "/sys/devices/")
}

func getHash(devPath string) string {
	hash := sha1.Sum([]byte(devPath))
	return hex.EncodeToString(hash[:])[:8]
}

func getBusType(devPath string) string {
	devPath = filepath.Join("/", devPath)
	devPath = strings.ToLower(devPath)

	for _, v := range []string{"i2c", "isa", "pci", "spi", "virtual", "acpi", "hid", "mdio", "scsi"} {
		if strings.Contains(devPath, "/"+v) {
			return v
		}
	}
	return "unk"
}

// shouldSkip indicates if a given filename should be skipped during the filesystem walk operation.
func shouldSkip(file string) bool {
	if strings.HasPrefix(file, "runtime_") {
		return true
	}

	switch file {
	case "async":
	case "autosuspend_delay_ms":
	case "control":
	case "driver_override":
	case "modalias":
	case "uevent":
	default:
		return false
	}

	return true
}
