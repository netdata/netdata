// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"strings"
)

type familyFlagValues struct {
	family  string
	sources []sourceEntry
}

func (f *familyFlagValues) String() string {
	return fmt.Sprintf("%d %s sources", len(f.sources), f.family)
}

func (f *familyFlagValues) Set(value string) error {
	source, err := parseSourceToken(f.family, value)
	if err != nil {
		return err
	}
	f.sources = append(f.sources, source)
	return nil
}

func parseSourceToken(family, value string) (sourceEntry, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return sourceEntry{}, fmt.Errorf("empty source token")
	}

	format := ""
	if before, after, ok := strings.Cut(value, "@"); ok {
		value = strings.TrimSpace(before)
		format = strings.ToLower(strings.TrimSpace(after))
	}

	provider, artifact, ok := strings.Cut(value, ":")
	if !ok {
		return sourceEntry{}, fmt.Errorf(
			"invalid source %q, expected provider:artifact or provider:artifact@format",
			value,
		)
	}

	source := sourceEntry{
		family:   family,
		provider: strings.ToLower(strings.TrimSpace(provider)),
		artifact: strings.ToLower(strings.TrimSpace(artifact)),
		format:   format,
	}

	if err := normalizeSourceEntry(&source); err != nil {
		return sourceEntry{}, err
	}
	if err := validateSourceEntry(source, "flag"); err != nil {
		return sourceEntry{}, err
	}
	return source, nil
}
