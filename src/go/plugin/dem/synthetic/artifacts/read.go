// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func (s *Store) Fetch(ctx context.Context, jobID, runID, artifactID string) (synthetic.Artifact, []byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return synthetic.Artifact{}, nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return synthetic.Artifact{}, nil, err
	}
	if !runIDPattern.MatchString(runID) || !artifactIDPattern.MatchString(artifactID) {
		return synthetic.Artifact{}, nil, ErrInvalidID
	}
	m, err := s.readOwnedManifest(jobID, runID)
	if err != nil {
		return synthetic.Artifact{}, nil, err
	}
	var found *synthetic.Artifact
	for i := range m.Artifacts {
		if m.Artifacts[i].ID == artifactID {
			found = &m.Artifacts[i]
			break
		}
	}
	if found == nil {
		return synthetic.Artifact{}, nil, ErrNotFound
	}
	if found.Bytes > FetchMaxBytes {
		return *found, nil, ErrTooLarge
	}
	file, err := openRegular(s.root, "runs/"+runID+"/"+artifactID)
	if errors.Is(err, fs.ErrNotExist) {
		return *found, nil, ErrNotFound
	}
	if err != nil {
		return *found, nil, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(&contextReader{
		ctx: ctx,
		r:   file,
	}, FetchMaxBytes+1))
	err = errors.Join(readErr, file.Close())
	if err != nil {
		return *found, nil, err
	}
	if int64(len(raw)) > FetchMaxBytes {
		return *found, nil, ErrTooLarge
	}
	sum := sha256.Sum256(raw)
	if int64(len(raw)) != found.Bytes || hex.EncodeToString(sum[:]) != found.SHA256 {
		return *found, nil, fmt.Errorf("artifact content does not match its manifest")
	}
	return *found, raw, nil
}

// Manifest returns immutable metadata without reading capture contents.
func (s *Store) Manifest(ctx context.Context, jobID, runID string) (Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Manifest{}, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if !runIDPattern.MatchString(runID) {
		return Manifest{}, ErrInvalidID
	}
	m, err := s.readOwnedManifest(jobID, runID)
	if err != nil {
		return Manifest{}, err
	}
	return m.Manifest, nil
}

func (s *Store) readManifest(id string) (manifest, error) {
	if s.uncertain[id] {
		return manifest{}, fmt.Errorf("artifact publication is uncertain")
	}
	if !runIDPattern.MatchString(id) {
		return manifest{}, ErrInvalidID
	}
	if _, err := s.root.Lstat("runs/" + id); errors.Is(err, fs.ErrNotExist) {
		return manifest{}, ErrNotFound
	} else if err != nil {
		return manifest{}, err
	}
	file, err := openRegular(s.root, "runs/"+id+"/manifest.json")
	if errors.Is(err, fs.ErrNotExist) {
		return manifest{}, fmt.Errorf("invalid artifact manifest: %w", err)
	}
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	err = errors.Join(json.NewDecoder(file).Decode(&m), file.Close())
	if err != nil {
		return manifest{}, err
	}
	if m.Version != 2 || m.RunID != id || !validJobID(m.JobID) || m.PublishedUS <= 0 || len(m.Artifacts) == 0 {
		return manifest{}, fmt.Errorf("invalid artifact manifest")
	}
	ids := make(map[string]bool)
	for _, a := range m.Artifacts {
		if !artifactIDPattern.MatchString(a.ID) || ids[a.ID] || a.Bytes < 0 || len(a.SHA256) != 64 ||
			!((a.Kind == "screenshot" && a.MIME == "image/png") || (a.Kind == "report" && a.MIME == "text/html")) {
			return manifest{}, fmt.Errorf("invalid artifact manifest entry")
		}
		ids[a.ID] = true
	}
	return m, nil
}

func (s *Store) readOwnedManifest(jobID, runID string) (manifest, error) {
	if !validJobID(jobID) {
		return manifest{}, ErrInvalidID
	}
	m, err := s.readManifest(runID)
	if err != nil {
		return manifest{}, err
	}
	if m.JobID != jobID {
		return manifest{}, ErrOwnerMismatch
	}
	return m, nil
}
