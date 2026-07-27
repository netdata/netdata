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
	"strings"
)

// engineSrcDir is the src/ tree of the agent under test. It defaults to the
// checkout the corpus lives in, which is the case that matters: in-tree, the
// engine and the corpus move together. QUERY_CORPUS_SRC overrides it for a
// run against a binary built from a DIFFERENT worktree (validating a fix
// branch), where the corpus checkout's enum is not the one that was compiled.
func engineSrcDir() (string, error) {
	dir := os.Getenv("QUERY_CORPUS_SRC")
	if dir == "" {
		dir = "../../src"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, "web/api/queries/query.h")); err != nil {
		return "", fmt.Errorf("engine source not found at %s (set QUERY_CORPUS_SRC): %w", abs, err)
	}
	return abs, nil
}

var (
	// a member of the enum, one per line, ignoring comments
	reGroupingEnumMember = regexp.MustCompile(`^\s*(RRDR_GROUPING_[A-Z0-9_]+)\s*(?:=\s*[^,]+)?,`)
	// the registry's canonical name and the value it stands for
	reRegistryName  = regexp.MustCompile(`\.name\s*=\s*"([^"]+)"`)
	reRegistryValue = regexp.MustCompile(`\.value\s*=\s*(RRDR_GROUPING_[A-Z0-9_]+)`)
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

	r := &groupingRoster{
		Canonical: map[string]string{},
		Aliases:   map[string][]string{},
	}

	// the enum, between `typedef enum rrdr_time_grouping {` and its close
	inEnum := false
	declared := map[string]bool{}
	for _, line := range strings.Split(string(enumBody), "\n") {
		if !inEnum {
			if strings.Contains(line, "typedef enum rrdr_time_grouping") {
				inEnum = true
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "}") {
			break
		}
		m := reGroupingEnumMember.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		c := m[1]
		// UNDEFINED is the "no grouping" answer and SENTINEL is the loop
		// bound - neither is a grouping anyone can ask for
		if c == "RRDR_GROUPING_UNDEFINED" || c == "RRDR_GROUPING_SENTINEL" {
			continue
		}
		if declared[c] {
			return nil, fmt.Errorf("grouping %s declared twice in the enum", c)
		}
		declared[c] = true
		r.Order = append(r.Order, c)
	}

	if len(r.Order) == 0 {
		return nil, fmt.Errorf("no groupings found in %s/web/api/queries/query.h", src)
	}

	// the registry: `.name = "x"` then, a few lines down, `.value = CONST`.
	// The first name a constant gets is its canonical one - that is the one
	// time_grouping_id2txt() echoes back.
	var pendingName string
	for _, line := range strings.Split(string(registry), "\n") {
		if m := reRegistryName.FindStringSubmatch(line); m != nil {
			pendingName = m[1]
			continue
		}
		m := reRegistryValue.FindStringSubmatch(line)
		if m == nil || pendingName == "" {
			continue
		}
		c, name := m[1], pendingName
		pendingName = ""

		if c == "RRDR_GROUPING_UNDEFINED" || c == "RRDR_GROUPING_SENTINEL" {
			// the registry terminator and the fallback entry
			continue
		}
		if !declared[c] {
			return nil, fmt.Errorf("registry offers %q for %s, which the enum does not declare", name, c)
		}
		if _, seen := r.Canonical[c]; seen {
			r.Aliases[c] = append(r.Aliases[c], name)
			continue
		}
		r.Canonical[c] = name
	}

	var unnamed []string
	for _, c := range r.Order {
		if r.Canonical[c] == "" {
			unnamed = append(unnamed, c)
		}
	}
	if len(unnamed) > 0 {
		return nil, fmt.Errorf("the enum declares %s but the registry gives no name for them - "+
			"an unnamed grouping cannot be reached through time_group= at all", strings.Join(unnamed, ", "))
	}

	return r, nil
}
