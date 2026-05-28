// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import "gopkg.in/yaml.v2"

func cloneConfig(in Config) Config {
	if len(in) == 0 {
		return nil
	}

	type plain Config
	bs, err := yaml.Marshal((plain)(in))
	if err != nil {
		return nil
	}

	var out Config
	if err := yaml.Unmarshal(bs, &out); err != nil {
		return nil
	}
	return out
}

func cloneStoreStatus(status StoreStatus) StoreStatus {
	out := status
	out.LastValidation = cloneValidationStatus(status.LastValidation)
	return out
}

func cloneValidationStatus(status *ValidationStatus) *ValidationStatus {
	if status == nil {
		return nil
	}
	copyStatus := *status
	return &copyStatus
}
