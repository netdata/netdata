// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"

	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// Function wraps functions.Function with dyncfg-specific accessor and helper methods.
type Function struct {
	fn functions.Function
}

// NewFunction creates a new dyncfg Function wrapper.
func NewFunction(fn functions.Function) Function {
	return Function{fn: fn}
}

// Fn returns the underlying functions.Function.
// Use this when passing to APIs that expect functions.Function.
func (f Function) Fn() functions.Function {
	return f.fn
}

// UID returns the function's unique identifier.
func (f Function) UID() string {
	return f.fn.UID
}

// Source returns the function's source field.
func (f Function) Source() string {
	return f.fn.Source
}

// Payload returns the function's payload.
func (f Function) Payload() []byte {
	return f.fn.Payload
}

// ContentType returns the function's content type.
func (f Function) ContentType() string {
	return f.fn.ContentType
}

// Command returns the dyncfg command from Args[1].
// Returns empty Command if args has fewer than 2 elements.
func (f Function) Command() Command {
	if len(f.fn.Args) < 2 {
		return ""
	}
	return Command(strings.ToLower(f.fn.Args[1]))
}

// ID returns the config ID from Args[0].
// Returns empty string if args is empty.
func (f Function) ID() string {
	if len(f.fn.Args) < 1 {
		return ""
	}
	return f.fn.Args[0]
}

// JobName returns the job name from Args[2] (used in add command).
// Returns empty string if args has fewer than 3 elements.
// Sanitizes the name by replacing spaces and colons with underscores.
func (f Function) JobName() string {
	if len(f.fn.Args) < 3 {
		return ""
	}
	name := strings.ReplaceAll(f.fn.Args[2], " ", "_")
	name = strings.ReplaceAll(name, ":", "_")
	return name
}

// User returns the user value from the Source field.
func (f Function) User() string {
	return f.SourceValue("user")
}

// SourceValue extracts a value from the Source field.
// Source format is "key1=value1,key2=value2,...".
func (f Function) SourceValue(key string) string {
	prefix := key + "="
	for _, part := range strings.Split(f.fn.Source, ",") {
		if v, ok := strings.CutPrefix(part, prefix); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// HasPayload returns true if the function has a non-empty payload.
func (f Function) HasPayload() bool {
	return len(f.fn.Payload) > 0
}

// ValidateArgs checks if the function has at least the required number of arguments.
// Returns an error with a descriptive message if validation fails.
func (f Function) ValidateArgs(required int) error {
	if len(f.fn.Args) < required {
		return fmt.Errorf("missing required arguments: need %d, got %d", required, len(f.fn.Args))
	}
	return nil
}

// ValidateHasPayload checks if the function has a payload.
// Returns an error if the payload is empty.
func (f Function) ValidateHasPayload() error {
	if !f.HasPayload() {
		return fmt.Errorf("missing configuration payload")
	}
	return nil
}

// UnmarshalPayload unmarshals the payload into dst based on ContentType.
// Uses JSON for "application/json", YAML otherwise.
func (f Function) UnmarshalPayload(dst any) error {
	if f.fn.ContentType == "application/json" {
		return f.unmarshalJSON(dst)
	}
	return f.unmarshalYAML(dst)
}

// unmarshalJSON unmarshals the payload as JSON into dst.
func (f Function) unmarshalJSON(dst any) error {
	if err := json.Unmarshal(f.fn.Payload, dst); err != nil {
		return fmt.Errorf("failed to unmarshal JSON payload: %w", err)
	}
	return nil
}

// unmarshalYAML unmarshals the payload as YAML into dst.
func (f Function) unmarshalYAML(dst any) error {
	if err := yaml.Unmarshal(f.fn.Payload, dst); err != nil {
		return fmt.Errorf("failed to unmarshal YAML payload: %w", err)
	}
	return nil
}

// IsContentTypeJSON returns true if the content type is JSON.
func (f Function) IsContentTypeJSON() bool {
	return f.fn.ContentType == "application/json"
}
