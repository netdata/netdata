// SPDX-License-Identifier: GPL-3.0-or-later

package faro

import (
	"regexp"
	"regexp/syntax"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Expected verdicts are isbot 5.2.2's own for the same strings.
func TestIsBotMatchesIsbot(t *testing.T) {
	cases := []struct {
		ua  string
		bot bool
	}{
		{
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/120.0.0.0 Safari/537.36",
			true,
		},
		{"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", true},
		{
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Chrome-Lighthouse",
			true,
		},
		{"curl/8.4.0", true},
		{
			"Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.2; +https://openai.com/gptbot)",
			true,
		},
		{
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			false,
		},
		{
			"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36",
			false,
		},
		{
			"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Mobile Safari/537.36 GSA/300.0",
			false,
		},
		{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36 Edg/131.0.0.0",
			false,
		},
		{"", false},
	}
	for _, c := range cases {
		if got := IsBot(c.ua); got != c.bot {
			t.Errorf("IsBot(%q) = %v, want %v", c.ua, got, c.bot)
		}
	}
}

// Real browsers and tools around the look-around entries (Google app,
// in-app webviews, Java clients); verdicts are isbot 5.2.2's.
func TestIsBotAgreesWithIsbotOnLookAroundCases(t *testing.T) {
	cases := []struct {
		ua  string
		bot bool
	}{
		{
			"Mozilla/5.0 (Linux; Android 14; Pixel 7 Build/UQ1A) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/120.0 Mobile Safari/537.36 GoogleApp/15.0",
			false,
		},
		{
			"Mozilla/5.0 (Linux; Android 13; SM-S918B; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/119.0 Mobile Safari/537.36 [FB_IAB/FB4A;FBAV/440.0;]",
			false,
		},
		{
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) GSA/290.0.593 Mobile/15E148 Safari/604.1",
			false,
		},
		{
			"Mozilla/5.0 (Linux; Android 12; Pixel 6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/118.0 Mobile Safari/537.36 google pixel",
			false,
		},
		{
			"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36",
			false,
		},
		{"Mozilla/5.0 (compatible; Google-InspectionTool/1.0)", true},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", true},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0", false},
		{
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
			false,
		},
		{
			"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/23.0 Chrome/115.0 Mobile Safari/537.36",
			false,
		},
		{
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36 OPR/106.0",
			false,
		},
		{"python-requests/2.31.0", true},
		{"Wget/1.21.4", true},
		{"Java/17.0.1", true},
		{"Apache-HttpClient/4.5.14 (Java/17.0.8)", true},
		{"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)", true},
		{"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)", true},
		{
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36 Edg/116.0.1938.81",
			false,
		},
		{
			"Mozilla/5.0 (iPad; CPU OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0 Mobile/15E148 Safari/604.1",
			false,
		},
		{
			"Mozilla/5.0 (Linux; Android 11; moto g) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Mobile Safari/537.36 DuckDuckGo/5",
			false,
		},
	}
	for _, c := range cases {
		if got := IsBot(c.ua); got != c.bot {
			t.Errorf("IsBot(%q) = %v, want %v", c.ua, got, c.bot)
		}
	}
}

// referenceIsBot is the classification as one case-folding expression over
// the original text. IsBot must agree with it for every input.
func referenceIsBot(ua string) bool {
	if ua == "" {
		return false
	}
	botOnce.Do(compileBots)
	if botFolded().MatchString(ua) {
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

// botEntrySample derives a text the expression matches, so every entry of a
// refreshed list is exercised without hand-written examples.
func botEntrySample(re *syntax.Regexp) string {
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune)
	case syntax.OpCharClass:
		return string(re.Rune[0])
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return "x"
	case syntax.OpCapture, syntax.OpPlus, syntax.OpAlternate:
		return botEntrySample(re.Sub[0])
	case syntax.OpRepeat:
		return strings.Repeat(botEntrySample(re.Sub[0]), re.Min)
	case syntax.OpConcat:
		var b strings.Builder
		for _, sub := range re.Sub {
			b.WriteString(botEntrySample(sub))
		}
		return b.String()
	default: // Optional parts and empty-width assertions add no text.
		return ""
	}
}

// botDifferentialInputs places each entry's sample at the start, inside and
// at the end of a User-Agent, next to word and non-word characters, in both
// letter cases, and beside the non-ASCII letters that fold to ASCII ones.
func botDifferentialInputs(t *testing.T) []string {
	samples := []string{
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"curl/8.4.0",
	}
	for _, entry := range botPatternsRE2 {
		re, err := syntax.Parse(entry, syntax.Perl)
		require.NoError(t, err)
		sample := botEntrySample(re)
		require.Truef(t, regexp.MustCompile(entry).MatchString(sample), "sample %q does not match entry %q", sample, entry)
		samples = append(samples, sample)
	}
	fold := strings.NewReplacer("s", "ſ", "S", "ſ", "k", "K", "i", "İ")
	var inputs []string
	for _, sample := range samples {
		for _, placed := range []string{
			sample,
			sample + "x",
			"x" + sample,
			sample + "/1.0",
			" " + sample + " ",
			"Mozilla/5.0 (" + sample + ") AppleWebKit/537.36",
			sample + "ſ",
			"K" + sample,
			sample + "İ",
			"é " + sample,
			fold.Replace(sample),
		} {
			inputs = append(inputs, placed, strings.ToUpper(placed), strings.ToLower(placed))
		}
	}
	return inputs
}

func TestIsBotMatchesFoldedExpression(t *testing.T) {
	var bots, nonASCII int
	for _, ua := range botDifferentialInputs(t) {
		want := referenceIsBot(ua)
		assert.Equalf(t, want, IsBot(ua), "IsBot(%q)", ua)
		if want {
			bots++
		}
		if !isASCII(ua) {
			nonASCII++
		}
	}
	// The comparison must exercise both verdicts and both input classes.
	assert.Positive(t, bots)
	assert.Positive(t, nonASCII)
}

// Lower-cased input can only match entries without upper-case literals.
func TestBotEntriesAreLowerCase(t *testing.T) {
	for _, entry := range botPatternsRE2 {
		for i := range len(entry) {
			if c := entry[i]; c >= 'A' && c <= 'Z' {
				assert.Truef(t, i > 0 && entry[i-1] == '\\', "entry %q has an upper-case literal", entry)
			}
		}
	}
}

func FuzzIsBotMatchesFoldedExpression(f *testing.F) {
	for _, ua := range []string{
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"curl/8.4.0",
		"Java/17.0.1",
		"ſcrape agentſ checK",
	} {
		f.Add(ua)
	}
	f.Fuzz(func(t *testing.T, ua string) {
		if got, want := IsBot(ua), referenceIsBot(ua); got != want {
			t.Fatalf("IsBot(%q) = %v, folded expression gives %v", ua, got, want)
		}
	})
}
