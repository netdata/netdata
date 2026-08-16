// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func assertInvalidTimeGroupCondition(t *testing.T, endpoint string, params url.Values, wantJSON, wantNoCache bool) {
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
	if wantJSON && !json.Valid(body) {
		t.Fatalf("%s returned invalid JSON body %q", endpoint, body)
	}
	if wantNoCache && resp.Header.Get("Cache-Control") != "no-cache, no-store, must-revalidate" {
		t.Fatalf("%s returned Cache-Control %q, want a non-cacheable error response",
			endpoint, resp.Header.Get("Cache-Control"))
	}
}

func TestWeightsRejectMalformedTimeGroupCondition(t *testing.T) {
	trackContract(t, "CASE-023/weights-invalid-options")
	assertInvalidTimeGroupCondition(t, "/api/v2/weights", url.Values{
		"time_group":         {"percentage-of-time"},
		"time_group_options": {"not-a-condition"},
	}, true, true)
}

func TestBadgeRejectMalformedTimeGroupCondition(t *testing.T) {
	trackContract(t, "CASE-023/badge-invalid-options")
	assertInvalidTimeGroupCondition(t, "/api/v1/badge.svg", url.Values{
		"group":         {"percentage-of-time"},
		"group_options": {"not-a-condition"},
	}, false, true)
}

func TestV1DataRejectMalformedTimeGroupCondition(t *testing.T) {
	trackContract(t, "CASE-023/v1-data-invalid-options")
	assertInvalidTimeGroupCondition(t, "/api/v1/data", url.Values{
		"group":         {"percentage-of-time"},
		"group_options": {"not-a-condition"},
	}, false, true)
}

func TestV2DataRejectMalformedTimeGroupCondition(t *testing.T) {
	trackContract(t, "CASE-023/v2-data-invalid-options")
	assertInvalidTimeGroupCondition(t, "/api/v2/data", url.Values{
		"time_group":         {"percentage-of-time"},
		"time_group_options": {"not-a-condition"},
	}, false, true)
}
