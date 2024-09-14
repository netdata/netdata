// SPDX-License-Identifier: GPL-3.0-or-later

package confopt

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type Duration time.Duration

func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

func (d Duration) String() string {
	return d.Duration().String()
}

func (d *Duration) UnmarshalYAML(unmarshal func(any) error) error {
	var s string

	if err := unmarshal(&s); err != nil {
		return err
	}

	if v, err := time.ParseDuration(s); err == nil {
		*d = Duration(v)
		return nil
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		*d = Duration(v * float64(time.Second))
		return nil
	}

	return fmt.Errorf("unparsable duration format '%s'", s)
}

func (d Duration) MarshalYAML() (any, error) {
	seconds := float64(d) / float64(time.Second)
	return seconds, nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	s := string(b)

	if v, err := time.ParseDuration(s); err == nil {
		*d = Duration(v)
		return nil
	}
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		*d = Duration(time.Duration(v) * time.Second)
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		*d = Duration(v * float64(time.Second))
		return nil
	}

	return fmt.Errorf("unparsable duration format '%s'", s)
}

func (d Duration) MarshalJSON() ([]byte, error) {
	seconds := float64(d) / float64(time.Second)
	return json.Marshal(seconds)
}
