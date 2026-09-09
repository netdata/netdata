// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const NormalDirectory = "normal"
const normalRunsFilename = "runs.json"

// Updating this small index activates a run only after a complete device file
// exists. Failed/empty startups leave the previous evidence-bearing run intact.
type NormalRuns struct {
	Current  string `json:"current"`
	Previous string `json:"previous,omitempty"`
}

type NormalFile struct {
	RunID          string `json:"run_id"`
	Previous       bool   `json:"previous"`
	RegistrationID uint64 `json:"registration_id"`
	Filename       string `json:"filename"`
}

func normalFilename(registration uint64) string { return fmt.Sprintf("device-%020d.zst", registration) }

func parseNormalFilename(name string) (registration uint64, ok bool) {
	number := strings.TrimSuffix(strings.TrimPrefix(name, "device-"), ".zst")
	registration, _ = strconv.ParseUint(number, 10, 64)
	return registration, registration != 0 && name == normalFilename(registration)
}

func ReadNormalRuns(directory string) (NormalRuns, error) {
	file, err := os.Open(filepath.Join(directory, NormalDirectory, normalRunsFilename))
	if errors.Is(err, os.ErrNotExist) {
		return NormalRuns{}, nil
	}
	if err != nil {
		return NormalRuns{}, err
	}
	defer file.Close()
	var runs NormalRuns
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&runs); err != nil {
		return runs, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return runs, errors.New("invalid normal run index trailer")
	}
	if !validNormalRun(runs.Current) || (runs.Previous != "" && (!validNormalRun(runs.Previous) || runs.Previous == runs.Current)) {
		return runs, errors.New("invalid normal run identity")
	}
	return runs, nil
}

func validNormalRun(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}

// ListNormalFiles reads only the committed run index and filenames. Temporary
// or unactivated files never become previous-run evidence by accident.
func ListNormalFiles(directory string) ([]NormalFile, error) {
	runs, err := ReadNormalRuns(directory)
	if err != nil {
		return nil, err
	}
	var result []NormalFile
	for _, run := range []string{runs.Current, runs.Previous} {
		if run == "" {
			continue
		}
		files, err := os.ReadDir(filepath.Join(directory, NormalDirectory, run))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, file := range files {
			registration, ok := parseNormalFilename(file.Name())
			if !file.Type().IsRegular() || !ok {
				continue
			}
			result = append(result, NormalFile{RunID: run, Previous: run == runs.Previous, RegistrationID: registration, Filename: file.Name()})
		}
	}
	return result, nil
}

func (p *Publisher) activateNormal(ctx context.Context) error {
	if p.normal.activated {
		return nil
	}
	runs, err := ReadNormalRuns(p.directory)
	if err != nil {
		return err
	}
	if runs.Current != p.runID {
		runs = NormalRuns{Current: p.runID, Previous: runs.Current}
		path := filepath.Join(p.directory, NormalDirectory, normalRunsFilename)
		err = writeAtomicFile(ctx, path, (*os.File).Close, func(w io.Writer) error { return json.NewEncoder(w).Encode(runs) }, p.rename)
		if err != nil {
			return err
		}
	}
	p.normal.activated, p.normal.prunePending = true, true
	return nil
}

func (p *Publisher) cleanupNormal(ctx context.Context) error {
	p.mu.Lock()
	retired := p.normal.retired
	p.normal.retired = nil
	p.mu.Unlock()
	var retry []normalRetirement
	var errs []error
	for _, file := range retired {
		if ctx.Err() != nil {
			retry = append(retry, file)
			continue
		}
		if err := p.removeRetiredNormal(file); err != nil {
			retry = append(retry, file)
			errs = append(errs, err)
		}
	}
	if len(retry) > 0 {
		p.mu.Lock()
		p.normal.retired = append(p.normal.retired, retry...)
		p.mu.Unlock()
	}
	if p.normal.prunePending && ctx.Err() == nil {
		if err := p.pruneNormalRuns(ctx); err != nil {
			errs = append(errs, err)
		} else {
			p.normal.prunePending = false
		}
	}
	return errors.Join(errs...)
}

func (p *Publisher) pruneNormalRuns(ctx context.Context) error {
	runs, err := ReadNormalRuns(p.directory)
	if err != nil {
		return err
	}
	root := filepath.Join(p.directory, NormalDirectory)
	directories, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	var errs []error
	for _, directory := range directories {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !directory.IsDir() || !validNormalRun(directory.Name()) || directory.Name() == runs.Current || directory.Name() == runs.Previous {
			continue
		}
		path := filepath.Join(root, directory.Name())
		files, err := os.ReadDir(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		remaining := false
		for _, file := range files {
			_, owned := parseNormalFilename(strings.TrimSuffix(file.Name(), ".tmp"))
			if !owned || !file.Type().IsRegular() {
				remaining = true
				continue
			}
			if err := p.remove(filepath.Join(path, file.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
				remaining = true
				errs = append(errs, err)
			}
		}
		if !remaining {
			if err := p.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
