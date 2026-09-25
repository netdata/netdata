// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// EnumSpec describes an Enum option. Implementations are usually empty structs.
type EnumSpec interface {
	// Values lists the allowed values in documentation order.
	Values() []string
	// Default is the value an empty or omitted option means; it must be one of Values.
	Default() string
}

// Enum is a string option restricted to its spec's values, one of which is the
// default. An empty value means the default: decoding an empty string stores
// the default and encoding an empty value writes it, so a blank form field or
// file value round-trips as the default. An omitted field is never decoded and
// stays empty, so read the effective value with Normalized.
//
// Values match exactly, without case folding or trimming. Declare the allowed
// values as untyped string constants so they compare with an Enum and still
// pass as plain strings.
type Enum[S EnumSpec] string

// Normalized returns the default for an empty value and the value otherwise.
func (e Enum[S]) Normalized() Enum[S] {
	if e == "" {
		var spec S
		return Enum[S](spec.Default())
	}
	return e
}

// Validate reports an error when the normalized value is not an allowed value.
func (e Enum[S]) Validate() error {
	var spec S
	values := spec.Values()
	if slices.Contains(values, string(e.Normalized())) {
		return nil
	}
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	choices := strings.Join(quoted, "")
	if n := len(quoted); n > 1 {
		choices = strings.Join(quoted[:n-1], ", ") + " or " + quoted[n-1]
	}
	return fmt.Errorf("must be %s, got %q", choices, string(e))
}

func (e Enum[S]) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(e.Normalized()))
}

func (e *Enum[S]) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	*e = Enum[S](s).Normalized()
	return nil
}

func (e Enum[S]) MarshalYAML() (any, error) {
	return string(e.Normalized()), nil
}

func (e *Enum[S]) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	*e = Enum[S](s).Normalized()
	return nil
}
