// SPDX-License-Identifier: GPL-3.0-or-later

// Package grouping owns application policy and aggregation of native snapshots.
package grouping

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
)

type Rule struct {
	Name  string  `yaml:"name" json:"name"`
	Match []Match `yaml:"match" json:"match"`
}
type Match struct {
	Comm    string `yaml:"comm,omitempty" json:"comm,omitempty"`
	Cmdline string `yaml:"cmdline,omitempty" json:"cmdline,omitempty"`
}
type clause struct{ comm, cmdline matcher.Matcher }
type rule struct {
	name    string
	clauses []clause
}
type groupKey struct{ kind, name string }

// Engine is owned by one collection loop. Only current group IDs persist.
type Engine struct {
	rules  []rule
	ids    map[groupKey]uint32
	nextID uint32
}

func New(rules []Rule) (*Engine, error) {
	e := &Engine{ids: make(map[groupKey]uint32)}
	names := make(map[string]bool)
	for _, r := range rules {
		if strings.TrimSpace(r.Name) == "" || len(r.Match) == 0 {
			return nil, fmt.Errorf("group requires a name and match clauses")
		}
		if names[r.Name] {
			return nil, fmt.Errorf("duplicate group name %q", r.Name)
		}
		names[r.Name] = true
		compiled := rule{name: r.Name}
		for _, m := range r.Match {
			if m.Comm == "" && m.Cmdline == "" {
				return nil, fmt.Errorf("group %q has an empty match clause", r.Name)
			}
			var c clause
			var err error
			if m.Comm != "" {
				c.comm, err = matcher.Parse(m.Comm)
				if err != nil {
					return nil, fmt.Errorf("group %q comm: %w", r.Name, err)
				}
			}
			if m.Cmdline != "" {
				c.cmdline, err = matcher.Parse(m.Cmdline)
				if err != nil {
					return nil, fmt.Errorf("group %q cmdline: %w", r.Name, err)
				}
			}
			compiled.clauses = append(compiled.clauses, c)
		}
		e.rules = append(e.rules, compiled)
	}
	return e, nil
}

// Aggregate takes an unpublished owned snapshot, assigning Application and
// normalizing rates once before publication. Missing member metrics invalidate
// the group field instead of reporting a partial sum as a complete measurement.
func (e *Engine) Aggregate(s *model.Snapshot) ([]model.Group, []model.Assignment, error) {
	if s == nil {
		return nil, nil, fmt.Errorf("nil process snapshot")
	}
	names, err := e.applications(s.Processes)
	if err != nil {
		return nil, nil, err
	}
	normalize(s)
	groups := make([]model.Group, 0)
	indexes := make(map[groupKey]int)
	live := make(map[groupKey]uint32)
	assignments := make([]model.Assignment, len(s.Processes))
	for i := range s.Processes {
		p := &s.Processes[i]
		p.Application = names[i]
		assignments[i].Key = p.Key
		keys := [3]groupKey{{"application", names[i]}, {"user", strconv.FormatUint(uint64(p.UID), 10)}, {"group", strconv.FormatUint(uint64(p.GID), 10)}}
		for axis, key := range keys {
			idx, ok := indexes[key]
			if !ok {
				id := e.ids[key]
				if id == 0 {
					if e.nextID == math.MaxUint32 {
						return nil, nil, fmt.Errorf("group ID space exhausted")
					}
					e.nextID++
					id = e.nextID
				}
				live[key] = id
				idx = len(groups)
				indexes[key] = idx
				groups = append(groups, model.Group{ID: id, Kind: key.kind, Name: key.name, Valid: p.Valid, UptimeMax: p.Values[model.Uptime]})
			}
			g := &groups[idx]
			assignments[i].Groups[axis] = g.ID
			g.Valid &= p.Valid
			for metric, value := range p.Values {
				if !p.Has(metric) {
					continue
				}
				switch metric {
				case model.Uptime:
					if g.Processes == 0 || value < g.Values[metric] {
						g.Values[metric] = value
					}
					g.UptimeMax = math.Max(g.UptimeMax, value)
				case model.FDLimitPercent, model.PSSAge:
					g.Values[metric] = math.Max(g.Values[metric], value)
				default:
					g.Values[metric] += value
				}
			}
			g.Processes++
		}
	}
	e.ids = live
	return groups, assignments, nil
}

func (e *Engine) applications(processes []model.Process) ([]string, error) {
	n := len(processes)
	byPID := make(map[int32]int, n)
	names := make([]string, n)
	display := make([]string, n)
	managers := make([]bool, n)
	for i, p := range processes {
		if _, exists := byPID[p.Key.PID]; exists {
			return nil, fmt.Errorf("duplicate PID %d", p.Key.PID)
		}
		byPID[p.Key.PID] = i
		display[i] = processName(p)
		managers[i] = isManager(p, display[i])
		cmdline := strings.ReplaceAll(strings.TrimRight(p.Cmdline, "\x00"), "\x00", " ")
		for _, r := range e.rules {
			for _, c := range r.clauses {
				if (c.comm == nil || c.comm.MatchString(p.Comm)) && (c.cmdline == nil || c.cmdline.MatchString(cmdline)) {
					names[i] = r.name
					break
				}
			}
			if names[i] != "" {
				break
			}
		}
	}
	// Per-scan memoization avoids stale PID reuse/exec decisions. A cycle chooses
	// its lowest PID, independent of input order. Every node is traversed once.
	visiting := make([]bool, n)
	path := make([]int, 0, n)
	for start := range processes {
		if names[start] != "" {
			continue
		}
		path = path[:0]
		cur := start
		name := ""
		for {
			if names[cur] != "" {
				name = names[cur]
				break
			}
			if visiting[cur] {
				root := cur
				for j := len(path) - 1; j >= 0; j-- {
					v := path[j]
					if processes[v].Key.PID < processes[root].Key.PID {
						root = v
					}
					if v == cur {
						break
					}
				}
				name = display[root]
				break
			}
			visiting[cur] = true
			path = append(path, cur)
			p := processes[cur]
			parent, ok := byPID[p.PPID]
			// A younger parent indicates PID reuse or a racing procfs scan.
			if managers[cur] || !ok || p.PPID <= 1 || managers[parent] || processes[parent].Key.StartTime > p.Key.StartTime {
				name = display[cur]
				if (p.Key.PID != 1 && p.PPID == 0) || (ok && (display[parent] == "kthread" || display[parent] == "kthreadd")) {
					name = "kernel"
				}
				break
			}
			cur = parent
		}
		for _, idx := range path {
			names[idx] = name
			visiting[idx] = false
		}
	}
	return names, nil
}
