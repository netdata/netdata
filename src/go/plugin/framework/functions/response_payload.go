// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import "encoding/json"

// BuildJSONPayload builds the standard JSON payload used by function terminal responses.
func BuildJSONPayload(code int, message string) []byte {
	if code >= 400 && code < 600 {
		bs, _ := json.Marshal(struct {
			Status       int    `json:"status"`
			ErrorMessage string `json:"errorMessage"`
		}{
			Status:       code,
			ErrorMessage: message,
		})
		return bs
	}

	bs, _ := json.Marshal(struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{
		Status:  code,
		Message: message,
	})
	return bs
}
