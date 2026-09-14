// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output/journal"
)

type jsonRetentionConfig struct {
	MaxSize     *string `yaml:"max_size,omitempty" json:"max_size"`
	MaxDuration *string `yaml:"max_duration,omitempty" json:"max_duration"`
	RotateSize  *string `yaml:"rotation_size,omitempty" json:"rotation_size"`
	RotateDur   *string `yaml:"rotation_duration,omitempty" json:"rotation_duration"`
}

func parseRetentionConfig(jc jsonRetentionConfig) (journal.Retention, error) {
	rc := journal.Retention{
		MaxSize:     nil,
		MaxDuration: nil,
		RotateSize:  nil,
		RotateDur:   nil,
	}

	if jc.MaxSize != nil {
		if *jc.MaxSize == "" || *jc.MaxSize == "null" {
			rc.MaxSize = nil
		} else {
			v, err := journal.ParseSize(*jc.MaxSize)
			if err != nil {
				return rc, fmt.Errorf("retention.max_size: %w", err)
			}
			rc.MaxSize = &v
		}
	} else {
		d := journal.DefaultMaxSize
		rc.MaxSize = &d
	}

	if jc.MaxDuration != nil {
		if *jc.MaxDuration == "" || *jc.MaxDuration == "null" {
			rc.MaxDuration = nil
		} else {
			d, err := journal.ParseDuration(*jc.MaxDuration)
			if err != nil {
				return rc, fmt.Errorf("retention.max_duration: %w", err)
			}
			rc.MaxDuration = &d
		}
	}

	if jc.RotateSize != nil {
		if *jc.RotateSize == "" || *jc.RotateSize == "null" {
			rc.RotateSize = nil
		} else {
			v, err := journal.ParseSize(*jc.RotateSize)
			if err != nil {
				return rc, fmt.Errorf("retention.rotation_size: %w", err)
			}
			rc.RotateSize = &v
		}
	}

	if jc.RotateDur != nil {
		if *jc.RotateDur == "" || *jc.RotateDur == "null" {
			d := time.Duration(0)
			rc.RotateDur = &d
		} else {
			d, err := journal.ParseDuration(*jc.RotateDur)
			if err != nil {
				return rc, fmt.Errorf("retention.rotation_duration: %w", err)
			}
			if d < 0 {
				return rc, fmt.Errorf("retention.rotation_duration: must be zero or positive")
			}
			rc.RotateDur = &d
		}
	}

	return rc, journal.ValidateRetention(rc)
}
