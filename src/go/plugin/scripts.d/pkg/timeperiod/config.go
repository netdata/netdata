// SPDX-License-Identifier: GPL-3.0-or-later

package timeperiod

// Config describes a time period definition in YAML/JSON.
type Config struct {
	Name    string       `yaml:"name" json:"name"`
	Alias   string       `yaml:"alias,omitempty" json:"alias"`
	Rules   []RuleConfig `yaml:"rules" json:"rules"`
	Exclude []string     `yaml:"exclude,omitempty" json:"exclude"`
}

// RuleConfig describes one rule entry (weekly, nth_weekday, date).
type RuleConfig struct {
	Type    string   `yaml:"type" json:"type"`
	Days    []string `yaml:"days,omitempty" json:"days"`
	Ranges  []string `yaml:"ranges" json:"ranges"`
	Weekday string   `yaml:"weekday,omitempty" json:"weekday"`
	Nth     int      `yaml:"nth,omitempty" json:"nth"`
	Dates   []string `yaml:"dates,omitempty" json:"dates"`
}

// DefaultPeriodName represents the implicit always-on period.
const DefaultPeriodName = "24x7"

// DefaultPeriodConfig returns the builtin 24x7 schedule.
func DefaultPeriodConfig() Config {
	return Config{
		Name:  DefaultPeriodName,
		Alias: "Always on",
		Rules: []RuleConfig{
			{
				Type:   "weekly",
				Days:   []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"},
				Ranges: []string{"00:00-24:00"},
			},
		},
	}
}
