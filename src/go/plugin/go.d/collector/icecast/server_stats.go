// SPDX-License-Identifier: GPL-3.0-or-later

package icecast

import (
	"encoding/json"
	"fmt"
)

type (
	serverStats struct {
		IceStats *struct {
			Source iceSource `json:"source"`
		} `json:"icestats"`
	}
	iceSource   []sourceStats
	sourceStats struct {
		ServerName  string `json:"server_name"`
		StreamStart string `json:"stream_start"`
		Listeners   int64  `json:"listeners"`
	}
)

func (i *iceSource) UnmarshalJSON(data []byte) error {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}

	switch v.(type) {
	case []any:
		type plain iceSource
		return json.Unmarshal(data, (*plain)(i))
	case map[string]any:
		var s sourceStats
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*i = []sourceStats{s}
	default:
		return fmt.Errorf("invalid source data type: expected array or object")
	}

	return nil
}
