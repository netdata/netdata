// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	snmpdiag "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/diagnostics"
)

type diagnosticInput struct {
	root      fs.FS
	directory *os.Root
	file      *os.File
	bundle    *bundleFS
	report    *bundleReport
}

type bundleReport struct {
	Root             string  `json:"root"`
	CollectionStatus *string `json:"collection_status"`
}

type diagnosticListing struct {
	Bundle    *bundleReport             `json:"bundle,omitempty"`
	Lifecycle bool                      `json:"lifecycle"`
	Topology  []snmpdiag.CheckpointFile `json:"topology_checkpoints"`
	Normal    []snmpdiag.NormalFile     `json:"normal_devices"`
	Errors    map[string]string         `json:"errors,omitempty"`
}

func openInput(filename string) (*diagnosticInput, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	input := &diagnosticInput{}
	if info.IsDir() {
		directory, err := os.OpenRoot(filename)
		if err != nil {
			return nil, err
		}
		input.directory = directory
		// Confine evidence and metadata through the same owned directory handle.
		root := directory.FS()
		for _, marker := range []string{"MANIFEST.json", bundleStatusPath, bundleDiagnosticPath} {
			if _, err := fs.Stat(root, marker); err == nil {
				input.root, err = fs.Sub(root, bundleDiagnosticPath)
				if err != nil {
					directory.Close()
					return nil, err
				}
				input.report = readBundleReport(root, ".")
				return input, nil
			} else if !errors.Is(err, fs.ErrNotExist) {
				directory.Close()
				return nil, err
			}
		}
		input.root = root
		return input, nil
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("input must be a regular file or directory")
	}
	for _, format := range []string{".tar.zst", ".tar.gz", ".zip"} {
		if !strings.HasSuffix(filename, format) {
			continue
		}
		bundle, err := openBundle(filename, format)
		if err != nil {
			return nil, err
		}
		input.bundle = bundle
		input.root, err = fs.Sub(bundle, path.Join(bundle.root, bundleDiagnosticPath))
		if err != nil {
			bundle.Close()
			return nil, err
		}
		input.report = readBundleReport(bundle, bundle.root)
		return input, nil
	}
	input.file, err = os.Open(filename)
	return input, err
}

func readBundleReport(root fs.FS, prefix string) *bundleReport {
	report := &bundleReport{Root: prefix}
	data, err := fs.ReadFile(root, path.Join(prefix, bundleStatusPath))
	if err == nil {
		status := string(data)
		report.CollectionStatus = &status
	}
	return report
}

func (input *diagnosticInput) Close() error {
	if input.directory != nil {
		return input.directory.Close()
	}
	if input.bundle != nil {
		return input.bundle.Close()
	}
	if input.file != nil {
		return input.file.Close()
	}
	return nil
}

func (input *diagnosticInput) list() diagnosticListing {
	result := diagnosticListing{Bundle: input.report, Errors: make(map[string]string)}
	info, err := fs.Stat(input.root, snmpdiag.LifecycleFilename)
	switch {
	case err == nil && info.Mode().IsRegular():
		result.Lifecycle = true
	case err == nil:
		result.Errors["lifecycle"] = "lifecycle evidence is not a regular file"
	case !errors.Is(err, fs.ErrNotExist):
		result.Errors["lifecycle"] = err.Error()
	}
	result.Topology, err = snmpdiag.ListCheckpointsFS(input.root)
	if err != nil {
		result.Errors["topology"] = err.Error()
	}
	result.Normal, err = snmpdiag.ListNormalFilesFS(input.root)
	if err != nil {
		result.Errors["normal"] = err.Error()
	}
	if input.report != nil && input.report.CollectionStatus == nil {
		result.Errors["collection_status"] = "collection status is missing or unreadable; evidence availability does not establish collection completeness"
	}
	return result
}

func (input *diagnosticInput) selectDocument(options commandOptions) (io.ReadCloser, error) {
	if input.file != nil {
		if options.lifecycle || options.normal || options.checkpoint != 0 {
			return nil, errors.New("evidence selectors require a diagnostics directory or support bundle")
		}
		// The input owns this file; the selected reader must not close it twice.
		return io.NopCloser(input.file), nil
	}
	filename, err := selectDocumentName(input.root, options)
	if err != nil {
		return nil, err
	}
	info, err := fs.Stat(input.root, filename)
	if err != nil {
		return nil, fmt.Errorf("selected evidence %q is unavailable: %w; use list to inspect availability and collection status", filename, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("selected evidence %q is not a regular file", filename)
	}
	return input.root.Open(filename)
}

func selectDocumentName(root fs.FS, options commandOptions) (string, error) {
	if options.lifecycle {
		return snmpdiag.LifecycleFilename, nil
	}
	if options.normal {
		files, err := snmpdiag.ListNormalFilesFS(root)
		if err != nil {
			return "", fmt.Errorf("normal evidence index: %w", err)
		}
		for _, file := range files {
			if file.Previous == options.previousRun && file.RegistrationID == options.registrationID {
				return path.Join(snmpdiag.NormalDirectory, file.RunID, file.Filename), nil
			}
		}
		return "", fmt.Errorf("normal device %d is not retained in the selected run; use list to inspect availability and collection status", options.registrationID)
	}
	entries, err := snmpdiag.ListCheckpointsFS(root)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", errors.New("input has no topology checkpoints; use list to inspect availability and collection status, or --lifecycle for lifecycle evidence")
	}
	sequence := options.checkpoint
	if sequence == 0 {
		sequence = entries[len(entries)-1].Sequence
	}
	for _, entry := range entries {
		if entry.Sequence == sequence {
			return path.Join(snmpdiag.TopologyDirectory, entry.Filename), nil
		}
	}
	return "", fmt.Errorf("checkpoint %d is not retained", sequence)
}
