// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"context"

	"github.com/gohugoio/hashstructure"
)

// CalcHash calculates a hash for any object using hashstructure.
// Used by discoverers to generate unique hashes for targets.
func CalcHash(obj any) (uint64, error) {
	return hashstructure.Hash(obj, nil)
}

// MapAny converts a map[string]string to map[string]any.
// Used by discoverers to convert labels/annotations for template evaluation.
func MapAny(src map[string]string) map[string]any {
	if src == nil {
		return nil
	}
	m := make(map[string]any, len(src))
	for k, v := range src {
		m[k] = v
	}
	return m
}

// SendTargetGroup sends a target group to the channel with context cancellation support.
// If tgg is nil, nothing is sent.
func SendTargetGroup(ctx context.Context, in chan<- []TargetGroup, tgg TargetGroup) {
	if tgg == nil {
		return
	}
	select {
	case <-ctx.Done():
	case in <- []TargetGroup{tgg}:
	}
}
