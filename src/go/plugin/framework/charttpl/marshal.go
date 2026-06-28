// SPDX-License-Identifier: GPL-3.0-or-later

package charttpl

import "gopkg.in/yaml.v2"

// MarshalTemplate validates the spec and serializes it with the same yaml
// library the decoder uses, so the emitted template round-trips through
// DecodeYAML unchanged. It runs Validate only — it does NOT apply the
// decode-time defaults — so a field left unset (for example a chart omitting
// type) is emitted unset, exactly as the caller assembled it.
func (s Spec) MarshalTemplate() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
