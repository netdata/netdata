// SPDX-License-Identifier: GPL-3.0-or-later
// Package synthetic defines shared workflow values and pure request validation.
package synthetic

import (
	"errors"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var secretName = regexp.MustCompile(`^DEM_SECRET_[A-Za-z0-9_]+$`)
var scriptExtension = regexp.MustCompile(`\.(?:[cm]?[jt]s)$`)

type Kind string

const (
	Journey    Kind = "journey"
	Lighthouse Kind = "lighthouse"
)

// Request is ephemeral. Script and resolved secrets never enter history.
type Request struct {
	Kind       Kind
	Name       string
	Script     string
	ScriptPath string
	URL        string
	Secrets    map[string]string
	Timeout    time.Duration
	Capture    bool
}

func (r Request) JobID() string { return string(r.Kind) + ":" + r.Name }

// ValidateRequest checks the execution contract without acquiring resources or reading files.
func ValidateRequest(r Request) error {
	if r.Timeout < time.Millisecond {
		return errors.New("synthetic timeout must be at least one millisecond")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("synthetic job name is required")
	}
	switch r.Kind {
	case Journey:
		if (r.Script == "") == (r.ScriptPath == "") {
			return errors.New("set exactly one of script and script_path")
		}
		if r.Script != "" && strings.TrimSpace(r.Script) == "" {
			return errors.New("script must not be blank")
		}
		if r.ScriptPath != "" && (!filepath.IsAbs(r.ScriptPath) || !scriptExtension.MatchString(r.ScriptPath)) {
			return errors.New("script_path must be an absolute JS or TS file")
		}
	case Lighthouse:
		u, err := url.Parse(r.URL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
			return errors.New("audit URL must be absolute HTTP(S) without credentials")
		}
	default:
		return errors.New("unsupported synthetic kind")
	}
	for name, value := range r.Secrets {
		if !secretName.MatchString(name) || strings.ContainsRune(value, 0) {
			return errors.New("invalid DEM_SECRET_ environment entry")
		}
	}
	return nil
}
