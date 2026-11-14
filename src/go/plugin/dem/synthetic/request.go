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

	"github.com/netdata/netdata/go/plugins/plugin/dem/internal/vnodeguid"
)

var secretName = regexp.MustCompile(`^DEM_SECRET_[A-Za-z0-9_]+$`)
var scriptExtension = regexp.MustCompile(`\.(?:[cm]?[jt]s)$`)

type Kind string

const (
	Journey    Kind = "journey"
	Lighthouse Kind = "lighthouse"
)

// Schedule identifies the selected occurrence and its original admission queue time.
type Schedule struct {
	DueUS    int64
	QueuedUS int64
}

// Request is ephemeral. Script and resolved secrets never enter history.
type Request struct {
	Kind                Kind
	Name                string
	VnodeGUID           string
	Script              string
	ScriptPath          string
	URL                 string
	Secrets             map[string]string
	Timeout             time.Duration
	BrowserErrorSummary bool
	Capture             bool
	Trace               bool
}

// JobID is the job's history and artifact identity: "<kind>:<name>".
func (r Request) JobID() string { return string(r.Kind) + ":" + r.Name }

// ValidJobID reports whether id is a JobID of a journey or Lighthouse job.
func ValidJobID(id string) bool {
	kind, name, ok := strings.Cut(id, ":")
	return ok && (Kind(kind) == Journey || Kind(kind) == Lighthouse) && strings.TrimSpace(name) != ""
}

// ValidateRequest checks the execution contract without acquiring resources or reading files.
func ValidateRequest(r Request) error {
	if err := vnodeguid.Validate(r.VnodeGUID); err != nil {
		return err
	}
	if r.Timeout < time.Millisecond {
		return errors.New("synthetic timeout must be at least one millisecond")
	}
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("synthetic job name is required")
	}
	if r.Trace && (r.Kind != Journey || !r.Capture) {
		return errors.New("trace requires a journey with capture enabled")
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
