// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

// Package ibmdriver handles automatic download and setup of IBM DB2 CLI drivers.
// The drivers are stored in the Netdata var lib directory (typically /var/lib/netdata/ibm-clidriver/)
// where the netdata user has write permissions.
package ibmdriver

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	clidriverVersion = "v11.5.9"
	clidriverBaseURL = "https://public.dhe.ibm.com/ibmdl/export/pub/software/data/db2/drivers/odbc_cli"
)

// GetDriverPath returns the path to the clidriver directory
func GetDriverPath() (string, error) {
	// Get the Netdata var lib directory from environment
	varLibDir := os.Getenv("NETDATA_LIB_DIR")
	if varLibDir == "" {
		// Fallback to common locations if env var not set
		candidates := []string{
			"/var/lib/netdata",
			"/opt/netdata/var/lib/netdata",
		}
		for _, dir := range candidates {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				varLibDir = dir
				break
			}
		}
		if varLibDir == "" {
			return "", fmt.Errorf("cannot find Netdata var lib directory (NETDATA_LIB_DIR not set)")
		}
	}
	
	// Create IBM driver directory under var lib
	driverPath := filepath.Join(varLibDir, "ibm-clidriver")
	
	return driverPath, nil
}

// EnsureDriver checks if the driver is installed and downloads it if necessary
func EnsureDriver() error {
	driverPath, err := GetDriverPath()
	if err != nil {
		return err
	}
	
	// Check if driver already exists
	clidriverPath := filepath.Join(driverPath, "clidriver")
	libPath := filepath.Join(clidriverPath, "lib")
	includePath := filepath.Join(clidriverPath, "include")
	
	if dirExists(libPath) && dirExists(includePath) {
		// Driver already installed
		return nil
	}
	
	// Ensure base directory exists
	if err := os.MkdirAll(driverPath, 0755); err != nil {
		return fmt.Errorf("failed to create driver directory: %w", err)
	}
	
	// Download and install driver
	fmt.Println("IBM DB2 CLI driver not found. Downloading...")
	
	url := getDownloadURL()
	if url == "" {
		return fmt.Errorf("unsupported platform: %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	
	return downloadAndExtract(url, driverPath)
}

// SetupEnvironment sets the required environment variables
func SetupEnvironment() error {
	driverPath, err := GetDriverPath()
	if err != nil {
		return err
	}
	
	// Set IBM_DB_HOME to the clidriver subdirectory
	clidriverPath := filepath.Join(driverPath, "clidriver")
	os.Setenv("IBM_DB_HOME", clidriverPath)
	
	// Update LD_LIBRARY_PATH (or equivalent on other platforms)
	libPath := filepath.Join(clidriverPath, "lib")
	
	switch runtime.GOOS {
	case "linux", "freebsd":
		currentLD := os.Getenv("LD_LIBRARY_PATH")
		if currentLD == "" {
			os.Setenv("LD_LIBRARY_PATH", libPath)
		} else {
			os.Setenv("LD_LIBRARY_PATH", libPath+":"+currentLD)
		}
	case "darwin":
		currentDY := os.Getenv("DYLD_LIBRARY_PATH")
		if currentDY == "" {
			os.Setenv("DYLD_LIBRARY_PATH", libPath)
		} else {
			os.Setenv("DYLD_LIBRARY_PATH", libPath+":"+currentDY)
		}
	case "windows":
		// On Windows, add to PATH
		currentPath := os.Getenv("PATH")
		os.Setenv("PATH", libPath+";"+currentPath)
	}
	
	return nil
}

func getDownloadURL() string {
	platform := ""
	
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			platform = "linuxx64_odbc_cli.tar.gz"
		case "ppc64le":
			platform = "ppc64le_odbc_cli.tar.gz"
		case "s390x":
			platform = "s390x_odbc_cli.tar.gz"
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			platform = "macos64_odbc_cli.tar.gz"
		case "arm64":
			// M1/M2 Macs
			platform = "macosarm64_odbc_cli.tar.gz"
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			platform = "ntx64_odbc_cli.zip"
		}
	case "aix":
		platform = "aix64_odbc_cli.tar.gz"
	}
	
	if platform == "" {
		return ""
	}
	
	return fmt.Sprintf("%s/%s/%s", clidriverBaseURL, clidriverVersion, platform)
}

func downloadAndExtract(url, destDir string) error {
	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}
	
	// Download file
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}
	
	// Create temporary file
	tmpFile, err := os.CreateTemp(destDir, "clidriver-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	
	// Download to temporary file
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return err
	}
	
	// Close and reopen for reading
	tmpFile.Close()
	tmpFile, err = os.Open(tmpFile.Name())
	if err != nil {
		return err
	}
	defer tmpFile.Close()
	
	// Extract based on file type
	if strings.HasSuffix(url, ".tar.gz") {
		return extractTarGz(tmpFile, destDir)
	} else if strings.HasSuffix(url, ".zip") {
		// TODO: Add zip extraction for Windows
		return fmt.Errorf("zip extraction not implemented yet")
	}
	
	return fmt.Errorf("unknown archive format")
}

func extractTarGz(r io.Reader, destDir string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()
	
	tr := tar.NewReader(gzr)
	
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		
		target := filepath.Join(destDir, header.Name)
		
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
	
	return nil
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}