// SPDX-License-Identifier: GPL-3.0-or-later

// The time-grouping roster, read from the engine's own source.
//
// The cross-grouping invariant sweep (layer 10) has to run EVERY grouping the
// engine offers, and the only way that survives someone adding a forty-third
// one is to take the list from the place they will add it: the
// RRDR_TIME_GROUPING enum, and the registry that gives each value the name
// `time_group=` accepts. A grouping added to the enum without a line in the
// sweep's table fails the sweep, loudly, naming the constant.
//
// There is no runtime endpoint that enumerates them - an unknown time_group
// silently falls back to `average` (query-group-over-time.c,
// time_grouping_parse), which is exactly why a missing one would otherwise
// pass unnoticed.
package corpus

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// engineSourceDir is resolved together with netdataBinary by TestMain.
var engineSourceDir string

func engineSrcDir() (string, error) {
	dir := engineSourceDir
	if dir == "" {
		var err error
		dir, err = filepath.Abs("../../src")
		if err != nil {
			return "", err
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "web/api/queries/query.h")); err != nil {
		return "", fmt.Errorf("engine source not found at %s: %w", dir, err)
	}
	return dir, nil
}

var (
	reGroupingEnum = regexp.MustCompile(
		`(?s)typedef\s+enum\s+rrdr_time_grouping\s*\{`)
	reGroupingEnumMember = regexp.MustCompile(
		`^\s*(RRDR_GROUPING_[A-Za-z0-9_]+)\b`)
	reGroupingRegistry = regexp.MustCompile(
		`(?s)\bapi_v1_data_groups\s*\[\s*\]\s*=\s*\{`)
	reRegistryName = regexp.MustCompile(
		`(?s)\.name\s*=\s*("(?:\\.|[^"\\])*")`)
	reRegistryValue = regexp.MustCompile(
		`(?s)\.value\s*=\s*(RRDR_GROUPING_[A-Za-z0-9_]+)\b`)
)

// groupingRoster is what the engine offers: every enum constant that names a
// real grouping, in declaration order, with the canonical name `time_group=`
// accepts for it and every alias that resolves to the same one.
type groupingRoster struct {
	Order     []string            // enum constants, in declaration order
	Canonical map[string]string   // constant -> the registry's FIRST name for it
	Aliases   map[string][]string // constant -> its further names
}

// readGroupingRoster parses the enum and the registry. Nothing here is
// allowed to guess: a constant with no registry entry, or a registry entry
// naming a constant that is not in the enum, is an error - both mean the two
// have drifted and the sweep would be testing a fiction.
func readGroupingRoster() (*groupingRoster, error) {
	src, err := engineSrcDir()
	if err != nil {
		return nil, err
	}

	enumBody, err := os.ReadFile(filepath.Join(src, "web/api/queries/query.h"))
	if err != nil {
		return nil, err
	}
	registry, err := os.ReadFile(filepath.Join(src, "web/api/queries/query-group-over-time.c"))
	if err != nil {
		return nil, err
	}

	r, err := parseGroupingRoster(enumBody, registry)
	if err != nil {
		return nil, fmt.Errorf("parse grouping roster from %s: %w", src, err)
	}
	return r, nil
}

func parseGroupingRoster(enumSource, registrySource []byte) (*groupingRoster, error) {
	enumText, err := stripCComments(string(enumSource))
	if err != nil {
		return nil, fmt.Errorf("enum source: %w", err)
	}
	registryText, err := stripCComments(string(registrySource))
	if err != nil {
		return nil, fmt.Errorf("registry source: %w", err)
	}

	enumBody, err := delimitedBody(enumText, reGroupingEnum)
	if err != nil {
		return nil, fmt.Errorf("time-grouping enum: %w", err)
	}
	registryBody, err := delimitedBody(registryText, reGroupingRegistry)
	if err != nil {
		return nil, fmt.Errorf("time-grouping registry: %w", err)
	}

	r := &groupingRoster{
		Canonical: map[string]string{},
		Aliases:   map[string][]string{},
	}
	declared := make(map[string]bool)
	for _, entry := range splitTopLevel(enumBody, ',') {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		match := reGroupingEnumMember.FindStringSubmatch(entry)
		if match == nil {
			if strings.Contains(entry, "RRDR_GROUPING_") {
				return nil, fmt.Errorf("cannot parse enum member %q", entry)
			}
			continue
		}
		constant := match[1]
		if constant == "RRDR_GROUPING_UNDEFINED" || constant == "RRDR_GROUPING_SENTINEL" {
			continue
		}
		if declared[constant] {
			return nil, fmt.Errorf("grouping %s declared twice in the enum", constant)
		}
		declared[constant] = true
		r.Order = append(r.Order, constant)
	}
	if len(r.Order) == 0 {
		return nil, fmt.Errorf("no requestable grouping constants found")
	}

	registryEntries, err := directBraceEntries(registryBody)
	if err != nil {
		return nil, fmt.Errorf("time-grouping registry entries: %w", err)
	}

	publicNames := make(map[string]string)
	for _, entry := range registryEntries {
		nameMatches := reRegistryName.FindAllStringSubmatch(entry, -1)
		valueMatches := reRegistryValue.FindAllStringSubmatch(entry, -1)
		if len(nameMatches) > 1 || len(valueMatches) > 1 {
			return nil, fmt.Errorf("registry entry has duplicate name or value fields: %q", entry)
		}
		if len(nameMatches) == 0 && len(valueMatches) == 0 {
			continue
		}

		constant := ""
		if len(valueMatches) == 1 {
			constant = valueMatches[0][1]
		}
		if constant == "RRDR_GROUPING_UNDEFINED" || constant == "RRDR_GROUPING_SENTINEL" {
			continue
		}
		if len(nameMatches) == 0 {
			return nil, fmt.Errorf("registry entry for %s has no string name", constant)
		}
		if constant == "" {
			name, _ := strconv.Unquote(nameMatches[0][1])
			return nil, fmt.Errorf("registry name %q has no grouping value", name)
		}

		name, err := strconv.Unquote(nameMatches[0][1])
		if err != nil || name == "" {
			return nil, fmt.Errorf("registry has invalid grouping name %q", nameMatches[0][1])
		}
		if !declared[constant] {
			return nil, fmt.Errorf(
				"registry offers %q for %s, which the enum does not declare",
				name, constant)
		}
		if previous, duplicate := publicNames[name]; duplicate {
			return nil, fmt.Errorf(
				"registry name %q is offered twice for %s and %s",
				name, previous, constant)
		}
		publicNames[name] = constant
		if _, seen := r.Canonical[constant]; seen {
			r.Aliases[constant] = append(r.Aliases[constant], name)
		} else {
			r.Canonical[constant] = name
		}
	}

	var unnamed []string
	for _, constant := range r.Order {
		if r.Canonical[constant] == "" {
			unnamed = append(unnamed, constant)
		}
	}
	if len(unnamed) > 0 {
		return nil, fmt.Errorf(
			"the enum declares %s but the registry gives no name for them",
			strings.Join(unnamed, ", "))
	}
	return r, nil
}

func delimitedBody(source string, marker *regexp.Regexp) (string, error) {
	match := marker.FindStringIndex(source)
	if match == nil {
		return "", fmt.Errorf("opening declaration not found")
	}
	open := strings.LastIndex(source[match[0]:match[1]], "{")
	if open < 0 {
		return "", fmt.Errorf("opening brace not found")
	}
	open += match[0]
	close, err := matchingBrace(source, open)
	if err != nil {
		return "", err
	}
	return source[open+1 : close], nil
}

func matchingBrace(source string, open int) (int, error) {
	depth := 0
	quote := byte(0)
	escaped := false
	for i := open; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("closing brace not found")
}

func directBraceEntries(source string) ([]string, error) {
	var entries []string
	for i := 0; i < len(source); {
		if source[i] != '{' {
			i++
			continue
		}
		close, err := matchingBrace(source, i)
		if err != nil {
			return nil, err
		}
		entries = append(entries, source[i+1:close])
		i = close + 1
	}
	return entries, nil
}

func splitTopLevel(source string, separator byte) []string {
	var entries []string
	start := 0
	depth := 0
	quote := byte(0)
	escaped := false
	for i := 0; i < len(source); i++ {
		c := source[i]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		switch c {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		default:
			if c == separator && depth == 0 {
				entries = append(entries, source[start:i])
				start = i + 1
			}
		}
	}
	entries = append(entries, source[start:])
	return entries
}

func stripCComments(source string) (string, error) {
	out := []byte(source)
	inBlock, inLine := false, false
	quote := byte(0)
	escaped := false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inLine {
			if c == '\n' {
				inLine = false
			} else {
				out[i] = ' '
			}
			continue
		}
		if inBlock {
			if c == '*' && i+1 < len(out) && out[i+1] == '/' {
				out[i], out[i+1] = ' ', ' '
				i++
				inBlock = false
			} else if c != '\n' {
				out[i] = ' '
			}
			continue
		}
		if quote != 0 {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == quote {
				quote = 0
			}
			continue
		}
		if c == '"' || c == '\'' {
			quote = c
			continue
		}
		if c == '/' && i+1 < len(out) {
			switch out[i+1] {
			case '/':
				out[i], out[i+1] = ' ', ' '
				i++
				inLine = true
			case '*':
				out[i], out[i+1] = ' ', ' '
				i++
				inBlock = true
			}
		}
	}
	if inBlock {
		return "", fmt.Errorf("unterminated block comment")
	}
	if quote != 0 {
		return "", fmt.Errorf("unterminated quoted literal")
	}
	return string(out), nil
}
