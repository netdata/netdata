// SPDX-License-Identifier: GPL-3.0-or-later

package cloudwatch

import "fmt"

type configYAMLKeySpec struct {
	children map[string]configYAMLKeySpec
	elem     *configYAMLKeySpec
	mapElem  *configYAMLKeySpec
	allowAny bool
}

var (
	credentialYAMLSpec = configYAMLKeySpec{children: map[string]configYAMLKeySpec{
		"type": {}, "access_key_id": {}, "secret_access_key": {}, "session_token": {},
	}}
	assumeRoleYAMLSpec = configYAMLKeySpec{children: map[string]configYAMLKeySpec{
		"role_arn": {}, "external_id": {},
	}}
	targetYAMLSpec = configYAMLKeySpec{children: map[string]configYAMLKeySpec{
		"name": {}, "credentials": {}, "assume_role": assumeRoleYAMLSpec,
	}}
	profileSelectorYAMLSpec = configYAMLKeySpec{children: map[string]configYAMLKeySpec{
		"defaults": {}, "include": {}, "exclude": {},
	}}
	ruleYAMLSpec = configYAMLKeySpec{children: map[string]configYAMLKeySpec{
		"name": {}, "targets": {}, "profiles": profileSelectorYAMLSpec, "regions": {},
		"filters": {}, "labels": {}, "series": {}, "query": {},
	}}
	tagYAMLSpec = configYAMLKeySpec{children: map[string]configYAMLKeySpec{
		"name": {}, "rename": {},
	}}
	cloudWatchConfigYAMLSpec = configYAMLKeySpec{children: map[string]configYAMLKeySpec{
		// Common framework-owned job keys may be present when a complete job is decoded.
		"name": {}, "module": {}, "labels": {allowAny: true}, "priority": {},
		"update_every": {}, "autodetection_retry": {}, "vnode": {},
		"credentials": {mapElem: &credentialYAMLSpec},
		"targets":     {elem: &targetYAMLSpec},
		"rules":       {elem: &ruleYAMLSpec},
		"discovery": {children: map[string]configYAMLKeySpec{
			"refresh_every": {}, "recently_active_only": {},
		}},
		"tags":         {elem: &tagYAMLSpec},
		"query_offset": {}, "timeout": {},
	}}
)

func (c *Config) UnmarshalYAML(unmarshal func(any) error) error {
	var raw any
	if err := unmarshal(&raw); err != nil {
		return err
	}
	if err := rejectUnknownConfigYAMLKeys(raw, cloudWatchConfigYAMLSpec, ""); err != nil {
		return err
	}

	type plain Config
	return unmarshal((*plain)(c))
}

func rejectUnknownConfigYAMLKeys(node any, spec configYAMLKeySpec, path string) error {
	if spec.allowAny || node == nil {
		return nil
	}

	switch value := node.(type) {
	case map[any]any:
		for rawKey, childValue := range value {
			key, ok := rawKey.(string)
			if !ok {
				return fmt.Errorf("%sconfig key %v is not a string", configPathPrefix(path), rawKey)
			}
			if err := rejectConfigYAMLMapEntry(key, childValue, spec, path); err != nil {
				return err
			}
		}
	case map[string]any:
		for key, childValue := range value {
			if err := rejectConfigYAMLMapEntry(key, childValue, spec, path); err != nil {
				return err
			}
		}
	case []any:
		if spec.elem == nil {
			return nil
		}
		for i, item := range value {
			if err := rejectUnknownConfigYAMLKeys(item, *spec.elem, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectConfigYAMLMapEntry(key string, value any, spec configYAMLKeySpec, path string) error {
	childPath := key
	if path != "" {
		childPath = path + "." + key
	}
	if spec.mapElem != nil {
		return rejectUnknownConfigYAMLKeys(value, *spec.mapElem, childPath)
	}
	if spec.children == nil {
		return nil
	}
	child, ok := spec.children[key]
	if !ok {
		return fmt.Errorf("%sunknown config key %q", configPathPrefix(path), key)
	}
	return rejectUnknownConfigYAMLKeys(value, child, childPath)
}

func configPathPrefix(path string) string {
	if path == "" {
		return ""
	}
	return path + ": "
}
