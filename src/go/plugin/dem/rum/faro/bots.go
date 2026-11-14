// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

// contextRule is an isbot entry that needs look-around: core must match
// without one of notBefore right before it or notAfter right after it.
type contextRule struct {
	core      *regexp.Regexp
	notBefore []string
	notAfter  []string
}

var (
	botOnce    sync.Once
	botEntries []*regexp.Regexp
	botRules   []contextRule
)

// botFolded is every plain entry as one case-folding expression. Matching it
// walks all alternatives for each input character, so it serves only input
// with non-ASCII bytes: lower-casing such text is not equivalent to folding
// (ſ, K and İ change which letters and word boundaries match).
var botFolded = sync.OnceValue(func() *regexp.Regexp {
	return regexp.MustCompile("(?i)" + strings.Join(botPatternsRE2, "|"))
})

func compileBots() {
	// Entries are lower-case; compiled separately they match lower-cased ASCII
	// text exactly like botFolded, and literal entries use substring search.
	botEntries = make([]*regexp.Regexp, len(botPatternsRE2))
	for i, entry := range botPatternsRE2 {
		botEntries[i] = regexp.MustCompile(entry)
	}
	rule := func(core string, notBefore, notAfter []string) contextRule {
		return contextRule{regexp.MustCompile(core), notBefore, notAfter}
	}
	botRules = []contextRule{
		rule(`news`, []string{"g"}, []string{"sapphire"}),
		rule(`google`, []string{" channel/", "google/"}, []string{"wv", "app", "/google", " pixel"}),
		rule(`bots?(?:\b|_)`, []string{" cu"}, nil),
		rule(`scan`, []string{"cam"}, nil),
		rule(`http`, []string{"lib"}, nil),
		rule(`java`, nil, []string{";"}),
	}
}

// IsBot reports whether a User-Agent belongs to a crawler, monitoring
// tool, headless browser or other automation (the isbot list).
func IsBot(ua string) bool {
	if ua == "" {
		return false
	}
	botOnce.Do(compileBots)
	lower := strings.ToLower(ua)
	if isASCII(ua) {
		for _, entry := range botEntries {
			if entry.MatchString(lower) {
				return true
			}
		}
	} else if botFolded().MatchString(ua) {
		return true
	}
	for _, r := range botRules {
		for _, m := range r.core.FindAllStringIndex(lower, -1) {
			if !endsWithAny(lower[:m[0]], r.notBefore) && !startsWithAny(lower[m[1]:], r.notAfter) {
				return true
			}
		}
	}
	return false
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func endsWithAny(s string, suffixes []string) bool {
	for _, x := range suffixes {
		if strings.HasSuffix(s, x) {
			return true
		}
	}
	return false
}

func startsWithAny(s string, prefixes []string) bool {
	for _, x := range prefixes {
		if strings.HasPrefix(s, x) {
			return true
		}
	}
	return false
}
