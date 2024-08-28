package lmsensors

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// A filesystem is an interface to a filesystem, used for testing.
type filesystem interface {
	ReadFile(filename string) (string, error)
	Readlink(name string) (string, error)
	Stat(name string) (os.FileInfo, error)
	WalkDir(root string, walkFn fs.WalkDirFunc) error
}

// A Scanner scans for Devices, so data can be read from their Sensors.
type Scanner struct {
	fs filesystem
}

// New creates a new Scanner.
func New() *Scanner {
	return &Scanner{
		fs: &systemFilesystem{},
	}
}

// Scan scans for Devices and their Sensors.
func (s *Scanner) Scan() ([]*Device, error) {
	paths, err := s.detectDevicePaths()
	if err != nil {
		return nil, err
	}

	var devices []*Device

	for _, rootPath := range paths {
		dev := &Device{}
		raw := make(map[string]map[string]string)

		// Walk filesystem paths to fetch devices and sensors
		err := s.fs.WalkDir(rootPath, func(path string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			if de.IsDir() || !de.Type().IsRegular() {
				if de.IsDir() && path != rootPath {
					return fs.SkipDir
				}
				return nil
			}

			// Skip some files that can't be read or don't provide useful sensor information
			file := filepath.Base(path)
			if shouldSkip(file) {
				return nil
			}

			s, err := s.fs.ReadFile(path)
			if err != nil {
				return nil
			}

			if file == "name" {
				dev.Name = s
				return nil
			}

			// Sensor names in format "sensor#_foo", e.g. "temp1_input"
			parts := strings.SplitN(file, "_", 2)
			if len(parts) != 2 {
				return nil
			}

			if _, ok := raw[parts[0]]; !ok {
				raw[parts[0]] = make(map[string]string)
			}

			raw[parts[0]][parts[1]] = s

			return nil
		})
		if err != nil {
			return nil, err
		}

		sensors, err := parseSensors(raw)
		if err != nil {
			return nil, err
		}

		dev.Sensors = sensors
		devices = append(devices, dev)
	}

	renameDevices(devices)

	return devices, nil
}

// renameDevices renames devices in place to prevent duplicate device names, and to number each device.
func renameDevices(devices []*Device) {
	nameCount := make(map[string]int)

	for i := range devices {
		name := devices[i].Name
		devices[i].Name = fmt.Sprintf("%s-%02d",
			name,
			nameCount[name],
		)
		nameCount[name]++
	}
}

// detectDevicePaths performs a filesystem walk to paths where devices may reside on Linux.
func (s *Scanner) detectDevicePaths() ([]string, error) {
	const lookPath = "/sys/class/hwmon"

	var paths []string
	err := s.fs.WalkDir(lookPath, func(path string, de os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if de.Type()&os.ModeSymlink == 0 {
			return nil
		}

		dest, err := s.fs.Readlink(path)
		if err != nil {
			return err
		}

		dest = filepath.Join(lookPath, filepath.Clean(dest))

		// Symlink destination has a file called name, meaning a sensor exists here and data can be retrieved
		fi, err := s.fs.Stat(filepath.Join(dest, "name"))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && fi.Mode().IsRegular() {
			paths = append(paths, dest)
			return nil
		}

		// Symlink destination has another symlink called device, which can be read and used to retrieve data
		device := filepath.Join(dest, "device")
		fi, err = s.fs.Stat(device)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			return nil
		}

		if fi.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		device, err = s.fs.Readlink(device)
		if err != nil {
			return err
		}

		dest = filepath.Join(dest, filepath.Clean(device))

		// Symlink destination has a file called name, meaning a sensor exists here and data can be retrieved
		if _, err := s.fs.Stat(filepath.Join(dest, "name")); err != nil {
			if !os.IsNotExist(err) {
				return err
			}
			return nil
		}

		paths = append(paths, dest)

		return nil
	})

	return paths, err
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

var _ filesystem = &systemFilesystem{}

// A systemFilesystem is a filesystem which uses operations on the host filesystem.
type systemFilesystem struct{}

func (fs *systemFilesystem) ReadFile(filename string) (string, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (fs *systemFilesystem) Readlink(name string) (string, error) {
	return os.Readlink(name)
}

func (fs *systemFilesystem) Stat(name string) (os.FileInfo, error) {
	return os.Stat(name)
}

func (fs *systemFilesystem) WalkDir(root string, walkFn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, walkFn)
}
