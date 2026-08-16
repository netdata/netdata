// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func assertInvalidTimeGroupCondition(t *testing.T, endpoint string, params url.Values) {
	t.Helper()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(td.BaseURL + endpoint + "?" + params.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(body), "Invalid time-group condition.") {
		t.Fatalf("%s returned HTTP %d body %q, want HTTP 400 with the invalid-condition diagnostic",
			endpoint, resp.StatusCode, body)
	}
}

func TestWeightsRejectMalformedTimeGroupCondition(t *testing.T) {
	trackContract(t, "CASE-023/weights-invalid-options")
	assertInvalidTimeGroupCondition(t, "/api/v2/weights", url.Values{
		"time_group":         {"percentage-of-time"},
		"time_group_options": {"not-a-condition"},
	})
}

func TestBadgeRejectMalformedTimeGroupCondition(t *testing.T) {
	trackContract(t, "CASE-023/badge-invalid-options")
	assertInvalidTimeGroupCondition(t, "/api/v1/badge.svg", url.Values{
		"group":         {"percentage-of-time"},
		"group_options": {"not-a-condition"},
	})
}
