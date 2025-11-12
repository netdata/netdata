// SPDX-License-Identifier: GPL-3.0-or-later

package timeperiod

import (
	"fmt"
	"strings"
	"time"
)

// Set contains compiled periods.
type Set struct {
	periods map[string]*Period
}

// Period represents a compiled schedule.
type Period struct {
	Name     string
	Alias    string
	rules    []rule
	excludes []*Period
}

type rule interface {
	Allows(time.Time) bool
}

type weeklyRule struct {
	perDay map[time.Weekday][]minuteRange
}

type nthWeekdayRule struct {
	weekday time.Weekday
	nth     int
	ranges  []minuteRange
}

type dateRule struct {
	dates map[string][]minuteRange // YYYY-MM-DD -> ranges
}

type minuteRange struct {
	start int
	end   int
}

// Compile builds a Set from raw configs.
func Compile(cfgs []Config) (*Set, error) {
	periods := make(map[string]*Period, len(cfgs))
	for _, cfg := range cfgs {
		if cfg.Name == "" {
			return nil, fmt.Errorf("time_period missing name")
		}
		if _, exists := periods[cfg.Name]; exists {
			return nil, fmt.Errorf("duplicate time_period '%s'", cfg.Name)
		}
		pr, err := compilePeriod(cfg)
		if err != nil {
			return nil, fmt.Errorf("time_period '%s': %w", cfg.Name, err)
		}
		periods[cfg.Name] = pr
	}
	// Resolve excludes
	for name, p := range periods {
		for _, exName := range cfgs[findConfigIndex(cfgs, name)].Exclude {
			ex, ok := periods[exName]
			if !ok {
				return nil, fmt.Errorf("time_period '%s': exclude '%s' not found", name, exName)
			}
			p.excludes = append(p.excludes, ex)
		}
	}
	return &Set{periods: periods}, nil
}

func findConfigIndex(cfgs []Config, name string) int {
	for i, cfg := range cfgs {
		if cfg.Name == name {
			return i
		}
	}
	return -1
}

func compilePeriod(cfg Config) (*Period, error) {
	if len(cfg.Rules) == 0 {
		return nil, fmt.Errorf("time_period '%s' needs at least one rule", cfg.Name)
	}
	pr := &Period{Name: cfg.Name, Alias: cfg.Alias}
	for _, rc := range cfg.Rules {
		var r rule
		var err error
		switch strings.ToLower(rc.Type) {
		case "weekly", "":
			r, err = compileWeeklyRule(rc)
		case "nth_weekday":
			r, err = compileNthWeekdayRule(rc)
		case "date":
			r, err = compileDateRule(rc)
		default:
			return nil, fmt.Errorf("unsupported rule type '%s'", rc.Type)
		}
		if err != nil {
			return nil, err
		}
		pr.rules = append(pr.rules, r)
	}
	return pr, nil
}

func compileWeeklyRule(rc RuleConfig) (rule, error) {
	if len(rc.Ranges) == 0 {
		return nil, fmt.Errorf("weekly rule requires ranges")
	}
	daySet := rc.Days
	if len(daySet) == 0 {
		daySet = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}
	}
	m := make(map[time.Weekday][]minuteRange)
	ranges, err := parseRanges(rc.Ranges)
	if err != nil {
		return nil, err
	}
	for _, d := range daySet {
		wd, err := parseWeekday(d)
		if err != nil {
			return nil, err
		}
		m[wd] = append([]minuteRange{}, ranges...)
	}
	return &weeklyRule{perDay: m}, nil
}

func compileNthWeekdayRule(rc RuleConfig) (rule, error) {
	if rc.Weekday == "" || rc.Nth <= 0 {
		return nil, fmt.Errorf("nth_weekday rule requires weekday and nth > 0")
	}
	if rc.Nth > 5 {
		return nil, fmt.Errorf("nth_weekday nth must be <= 5")
	}
	ranges, err := parseRanges(rc.Ranges)
	if err != nil {
		return nil, err
	}
	wd, err := parseWeekday(rc.Weekday)
	if err != nil {
		return nil, err
	}
	return &nthWeekdayRule{weekday: wd, nth: rc.Nth, ranges: ranges}, nil
}

func compileDateRule(rc RuleConfig) (rule, error) {
	if len(rc.Dates) == 0 {
		return nil, fmt.Errorf("date rule requires dates")
	}
	ranges, err := parseRanges(rc.Ranges)
	if err != nil {
		return nil, err
	}
	m := make(map[string][]minuteRange)
	for _, ds := range rc.Dates {
		if _, err := time.Parse("2006-01-02", ds); err != nil {
			return nil, fmt.Errorf("invalid date '%s'", ds)
		}
		key := ds
		m[key] = append([]minuteRange{}, ranges...)
	}
	return &dateRule{dates: m}, nil
}

func parseRanges(list []string) ([]minuteRange, error) {
	if len(list) == 0 {
		return nil, fmt.Errorf("at least one range is required")
	}
	res := make([]minuteRange, 0, len(list))
	for _, item := range list {
		parts := strings.Split(item, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range '%s'", item)
		}
		s, err := parseMinute(parts[0])
		if err != nil {
			return nil, err
		}
		e, err := parseMinute(parts[1])
		if err != nil {
			return nil, err
		}
		if e < s {
			return nil, fmt.Errorf("range '%s' end before start", item)
		}
		res = append(res, minuteRange{start: s, end: e})
	}
	return res, nil
}

func parseMinute(val string) (int, error) {
	parts := strings.Split(val, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid time '%s'", val)
	}
	hour, err := parseIntBound(parts[0], 0, 24)
	if err != nil {
		return 0, err
	}
	min, err := parseIntBound(parts[1], 0, 59)
	if err != nil {
		return 0, err
	}
	if hour == 24 && min != 0 {
		return 0, fmt.Errorf("24:%02d is invalid", min)
	}
	return hour*60 + min, nil
}

func parseIntBound(val string, min, max int) (int, error) {
	var x int
	_, err := fmt.Sscanf(val, "%d", &x)
	if err != nil {
		return 0, fmt.Errorf("invalid number '%s'", val)
	}
	if x < min || x > max {
		return 0, fmt.Errorf("value '%s' out of bounds", val)
	}
	return x, nil
}

func parseWeekday(val string) (time.Weekday, error) {
	switch strings.ToLower(val) {
	case "sunday":
		return time.Sunday, nil
	case "monday":
		return time.Monday, nil
	case "tuesday":
		return time.Tuesday, nil
	case "wednesday":
		return time.Wednesday, nil
	case "thursday":
		return time.Thursday, nil
	case "friday":
		return time.Friday, nil
	case "saturday":
		return time.Saturday, nil
	default:
		return time.Sunday, fmt.Errorf("invalid weekday '%s'", val)
	}
}

// Resolve returns the compiled period for a name.
func (s *Set) Resolve(name string) (*Period, error) {
	if s == nil || name == "" {
		return nil, nil
	}
	per, ok := s.periods[name]
	if !ok {
		return nil, fmt.Errorf("time_period '%s' not defined", name)
	}
	return per, nil
}

// Allows determines whether the time falls inside the period (excluding child exclusions).
func (p *Period) Allows(t time.Time) bool {
	return p.allows(t, make(map[*Period]bool))
}

func (p *Period) allows(t time.Time, stack map[*Period]bool) bool {
	if p == nil {
		return true
	}
	if stack[p] {
		return false
	}
	stack[p] = true
	defer delete(stack, p)

	allowed := false
	for _, r := range p.rules {
		if r.Allows(t) {
			allowed = true
			break
		}
	}
	if !allowed {
		return false
	}
	for _, ex := range p.excludes {
		if ex == nil || ex == p {
			continue
		}
		if ex.allows(t, stack) {
			return false
		}
	}
	return true
}

// NextAllowed returns the next timestamp at or after t that is allowed.
func (p *Period) NextAllowed(t time.Time) time.Time {
	if p == nil {
		return t
	}
	for i := 0; i < 60*24*90; i++ { // search up to ~90 days
		if p.Allows(t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

// rule implementations

func (r *weeklyRule) Allows(t time.Time) bool {
	if r == nil {
		return false
	}
	min := t.Hour()*60 + t.Minute()
	list := r.perDay[t.Weekday()]
	for _, rng := range list {
		if rng.contains(min) {
			return true
		}
	}
	return false
}

func (mr minuteRange) contains(min int) bool {
	return min >= mr.start && min < mr.end
}

func (r *nthWeekdayRule) Allows(t time.Time) bool {
	if r == nil {
		return false
	}
	if t.Weekday() != r.weekday {
		return false
	}
	day := t.Day()
	nth := (day-1)/7 + 1
	if nth != r.nth {
		return false
	}
	min := t.Hour()*60 + t.Minute()
	for _, rng := range r.ranges {
		if rng.contains(min) {
			return true
		}
	}
	return false
}

func (r *dateRule) Allows(t time.Time) bool {
	if r == nil {
		return false
	}
	key := t.Format("2006-01-02")
	list := r.dates[key]
	if len(list) == 0 {
		return false
	}
	min := t.Hour()*60 + t.Minute()
	for _, rng := range list {
		if rng.contains(min) {
			return true
		}
	}
	return false
}

// EnsureDefault appends the builtin period when missing.
func EnsureDefault(cfgs []Config) []Config {
	for _, cfg := range cfgs {
		if cfg.Name == DefaultPeriodName {
			return cfgs
		}
	}
	return append(cfgs, DefaultPeriodConfig())
}
