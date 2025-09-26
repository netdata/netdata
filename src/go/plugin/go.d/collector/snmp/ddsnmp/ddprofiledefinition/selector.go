package ddprofiledefinition

import (
	"regexp"
	"slices"
	"strings"
)

type (
	SelectorSpec []SelectorRule

	SelectorRule struct {
		SysObjectID SelectorIncludeExclude `yaml:"sysobjectid,omitempty" json:"sysobjectid,omitempty"`
		SysDescr    SelectorIncludeExclude `yaml:"sysdescr,omitempty"    json:"sysdescr,omitempty"`
	}

	SelectorIncludeExclude struct {
		Include StringArray `yaml:"include,omitempty" json:"include,omitempty"`
		Exclude StringArray `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	}
)

func (s SelectorSpec) Clone() SelectorSpec {
	if len(s) == 0 {
		return nil
	}
	out := make(SelectorSpec, len(s))
	for i := range s {
		out[i] = s[i].clone()
	}
	return out
}

func (r SelectorRule) clone() SelectorRule {
	return SelectorRule{
		SysObjectID: r.SysObjectID.clone(),
		SysDescr:    r.SysDescr.clone(),
	}
}

func (ie SelectorIncludeExclude) clone() SelectorIncludeExclude {
	return SelectorIncludeExclude{
		Include: slices.Clone(ie.Include),
		Exclude: slices.Clone(ie.Exclude),
	}
}

func (s SelectorSpec) HasExactOidMatch(deviceSysObjectID string) bool {
	for _, rule := range s {
		if slices.ContainsFunc(rule.SysObjectID.Include, func(s string) bool { return deviceSysObjectID == s }) {
			return true
		}
	}
	return false
}

func (s SelectorSpec) Matches(deviceSysObjectID, deviceSysDescr string) (bool, string) {
	for _, rule := range s {
		if ok, matchedPrefix := rule.Matches(deviceSysObjectID, deviceSysDescr); ok {
			return true, matchedPrefix
		}
	}
	return false, ""
}

func (r SelectorRule) Matches(deviceSysObjectID, deviceSysDescr string) (bool, string) {
	if len(r.SysObjectID.Include) == 0 && len(r.SysDescr.Include) == 0 {
		return false, ""
	}

	// Excludes first
	if slices.ContainsFunc(r.SysObjectID.Exclude, func(s string) bool {
		return SelectorOidMatches(deviceSysObjectID, s)
	}) {
		return false, ""
	}
	lo := strings.ToLower(deviceSysDescr)
	if slices.ContainsFunc(r.SysDescr.Exclude, func(s string) bool {
		return strings.Contains(lo, strings.ToLower(s))
	}) {
		return false, ""
	}

	// Includes: OID (pick longest selector)
	var longestMatch string
	if len(r.SysObjectID.Include) > 0 {
		for _, sel := range r.SysObjectID.Include {
			if SelectorOidMatches(deviceSysObjectID, sel) && len(sel) > len(longestMatch) {
				longestMatch = sel
				if IsPlainOid(longestMatch) {
					break
				}
			}
		}
		if longestMatch == "" {
			return false, ""
		}
	}

	// Includes: sysDescr
	if len(r.SysDescr.Include) > 0 {
		if !slices.ContainsFunc(r.SysDescr.Include, func(s string) bool {
			return strings.Contains(lo, strings.ToLower(s))
		}) {
			return false, ""
		}
	}

	return true, longestMatch
}

var rePlainOid = regexp.MustCompile(`^[0-9]+(\.[0-9]+)*$`)

func SelectorOidMatches(deviceSysObjId, selectorOid string) bool {
	if IsPlainOid(selectorOid) {
		return deviceSysObjId == selectorOid
	}
	ok, err := regexp.MatchString(selectorOid, deviceSysObjId)
	return err == nil && ok
}

func IsPlainOid(selectorOid string) bool {
	return rePlainOid.MatchString(selectorOid)
}
