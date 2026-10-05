// SPDX-License-Identifier: GPL-3.0-or-later

package beacon

import "testing"

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
