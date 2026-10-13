// SPDX-License-Identifier: GPL-3.0-or-later

package config

// Capture selects optional evidence independently of measurement/detail sampling.
type Capture struct {
	// Nil means country; an explicit empty string is invalid.
	Geolocation        *string `yaml:"geolocation,omitempty" json:"geolocation"`
	FrustrationSignals bool    `yaml:"frustration_signals" json:"frustration_signals"`
}

const (
	GeolocationOff     = "off"
	GeolocationCountry = "country"
	GeolocationCity    = "city"
)

func (s Site) GeolocationMode() string {
	if s.Capture == nil || s.Capture.Geolocation == nil {
		return GeolocationCountry
	}
	return *s.Capture.Geolocation
}

func (s Site) FrustrationSignalsOn() bool {
	return s.Capture != nil && s.Capture.FrustrationSignals
}
