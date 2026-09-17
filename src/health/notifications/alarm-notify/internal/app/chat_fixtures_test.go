// SPDX-License-Identifier: GPL-3.0-or-later

package app

type pushoverMessage struct {
	Token     string `json:"token"`
	User      string `json:"user"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	HTML      int    `json:"html"`
	Priority  int    `json:"priority"`
	Timestamp int64  `json:"timestamp"`
	URL       string `json:"url,omitempty"`
	URLTitle  string `json:"url_title,omitempty"`
}

type gotifyMessage struct {
	Title    string `json:"title"`
	Message  string `json:"message"`
	Priority int    `json:"priority"`
}

const pushoverTestToken = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
const pushoverTestUser = "UUUUUUUUUUUUUUUUUUUUUUUUUUUUUU"

func teamsMatrixTestDestination(provider string) map[string]any {
	if provider == "msteams" {
		return map[string]any{"type": provider, "url": "https://example.com/teams?sig=synthetic-url-secret"}
	}
	return map[string]any{"type": provider, "api_url": "https://example.com/matrix/", "access_token": "synthetic-token", "room_id": "!room:example.org"}
}

type ntfyAction struct {
	Action string `json:"action"`
	Label  string `json:"label"`
	URL    string `json:"url"`
	Clear  bool   `json:"clear"`
}
