// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"fmt"
	"time"
)

type jsonRetentionConfig struct {
	MaxSize     *string `yaml:"max_size,omitempty" json:"max_size"`
	MaxDuration *string `yaml:"max_duration,omitempty" json:"max_duration"`
	RotateSize  *string `yaml:"rotation_size,omitempty" json:"rotation_size"`
	RotateDur   *string `yaml:"rotation_duration,omitempty" json:"rotation_duration"`
}

func (rc RetentionConfig) toJSON() jsonRetentionConfig {
	jc := jsonRetentionConfig{}
	if rc.MaxSize != nil {
		s := formatHumanSize(*rc.MaxSize)
		jc.MaxSize = &s
	}
	if rc.MaxDuration != nil {
		if *rc.MaxDuration > 0 {
			s := humanDuration(*rc.MaxDuration)
			jc.MaxDuration = &s
		}
	}
	if rc.RotateSize != nil {
		s := formatHumanSize(*rc.RotateSize)
		jc.RotateSize = &s
	}
	if rc.RotateDur != nil {
		s := humanDuration(*rc.RotateDur)
		jc.RotateDur = &s
	}
	return jc
}

func parseRetentionConfig(jc jsonRetentionConfig) (RetentionConfig, error) {
	rc := RetentionConfig{
		MaxSize:     nil,
		MaxDuration: nil,
		RotateSize:  nil,
		RotateDur:   nil,
	}

	if jc.MaxSize != nil {
		if *jc.MaxSize == "" || *jc.MaxSize == "null" {
			rc.MaxSize = nil
		} else {
			v, err := parseHumanSize(*jc.MaxSize)
			if err != nil {
				return rc, fmt.Errorf("retention.max_size: %w", err)
			}
			rc.MaxSize = &v
		}
	} else {
		d := defaultMaxSize
		rc.MaxSize = &d
	}

	if jc.MaxDuration != nil {
		if *jc.MaxDuration == "" || *jc.MaxDuration == "null" {
			rc.MaxDuration = nil
		} else {
			d, err := parseHumanDuration(*jc.MaxDuration)
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
			v, err := parseHumanSize(*jc.RotateSize)
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
			d, err := parseHumanDuration(*jc.RotateDur)
			if err != nil {
				return rc, fmt.Errorf("retention.rotation_duration: %w", err)
			}
			if d < 0 {
				return rc, fmt.Errorf("retention.rotation_duration: must be zero or positive")
			}
			rc.RotateDur = &d
		}
	}

	return rc, validateRetention(rc)
}

func (rc RetentionConfig) makeJournalConfig() JournalConfig {
	jc := JournalConfig{
		MaxSize:    rc.EffectiveMaxSize(),
		RotateSize: rc.EffectiveRotateSize(),
		RotateDur:  rc.EffectiveRotateDur(),
	}
	if d := rc.EffectiveMaxDuration(); d > 0 {
		jc.MaxDuration = d
	}
	return jc
}
