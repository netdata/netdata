// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import (
	"regexp"
	"strings"
	"sync"
)

// BotPatternJS is the bot pattern the snippet tests navigator.userAgent
// against before starting Faro.
func BotPatternJS() string { return botPatternJS }

// contextRule is an isbot entry that needs look-around: core must match
// without one of notBefore right before it or notAfter right after it.
type contextRule struct {
	core      *regexp.Regexp
	notBefore []string
	notAfter  []string
}

var (
	botOnce  sync.Once
	botPlain *regexp.Regexp
	botRules []contextRule
)

func compileBots() {
	botPlain = regexp.MustCompile("(?i)" + strings.Join(botPatternsRE2, "|"))
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
	if botPlain.MatchString(ua) {
		return true
	}
	lower := strings.ToLower(ua)
	for _, r := range botRules {
		for _, m := range r.core.FindAllStringIndex(lower, -1) {
			if !endsWithAny(lower[:m[0]], r.notBefore) && !startsWithAny(lower[m[1]:], r.notAfter) {
				return true
			}
		}
	}
	return false
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
