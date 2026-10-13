// SPDX-License-Identifier: GPL-3.0-or-later

package topologyv1

import (
	"fmt"
	"regexp"
)

var topologyIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.:-]*$`)

func validateNotifications(data map[string]any) error {
	raw, present := data["notifications"]
	if !present {
		return nil
	}

	notifications, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("data.notifications is not an array")
	}
	for i, rawNotification := range notifications {
		path := fmt.Sprintf("data.notifications[%d]", i)
		notification, ok := rawNotification.(map[string]any)
		if !ok {
			return fmt.Errorf("%s is not an object", path)
		}
		if err := validateNotification(path, notification); err != nil {
			return err
		}
	}
	return nil
}

func validateNotification(path string, notification map[string]any) error {
	allowed := map[string]struct{}{
		"severity":         {},
		"code":             {},
		"message":          {},
		"origin":           {},
		"affected_node_id": {},
	}
	if err := rejectUnknownFields(path, notification, allowed); err != nil {
		return err
	}
	severity, err := requiredNotificationField(path, notification, "severity")
	if err != nil {
		return err
	}
	if _, err := requiredEnum(path+".severity", severity,
		NotificationSeverityInfo, NotificationSeverityWarning, NotificationSeverityError); err != nil {
		return err
	}
	code, err := requiredNotificationField(path, notification, "code")
	if err != nil {
		return err
	}
	if err := validateTopologyID(path+".code", code); err != nil {
		return err
	}
	message, err := requiredNotificationField(path, notification, "message")
	if err != nil {
		return err
	}
	if err := validateRequiredString(path+".message", message); err != nil {
		return err
	}
	if origin, present := notification["origin"]; present {
		if err := validateNotificationOrigin(path+".origin", origin); err != nil {
			return err
		}
	}
	if affectedNodeID, present := notification["affected_node_id"]; present {
		if _, ok := affectedNodeID.(string); !ok {
			return fmt.Errorf("%s.affected_node_id is not a string", path)
		}
	}
	return nil
}

func validateNotificationOrigin(path string, raw any) error {
	origin, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("%s is not an object", path)
	}
	allowed := map[string]struct{}{
		"source":        {},
		"instance":      {},
		"node_id":       {},
		"machine_guid":  {},
		"agent_version": {},
		"plugin":        {},
		"capabilities":  {},
	}
	if err := rejectUnknownFields(path, origin, allowed); err != nil {
		return err
	}
	source, err := requiredNotificationField(path, origin, "source")
	if err != nil {
		return err
	}
	if err := validateTopologyID(path+".source", source); err != nil {
		return err
	}
	instance, err := requiredNotificationField(path, origin, "instance")
	if err != nil {
		return err
	}
	if _, ok := instance.(string); !ok {
		return fmt.Errorf("%s.instance is not a string", path)
	}
	for _, field := range []string{"node_id", "machine_guid", "agent_version", "plugin"} {
		if value, present := origin[field]; present {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%s.%s is not a string", path, field)
			}
		}
	}
	if rawCapabilities, present := origin["capabilities"]; present {
		capabilities, err := collectStringList(path+".capabilities", rawCapabilities, false)
		if err != nil {
			return err
		}
		for i, capability := range capabilities {
			if !topologyIDPattern.MatchString(capability) {
				return fmt.Errorf("%s.capabilities[%d] is not a valid identifier", path, i)
			}
		}
	}
	return nil
}

func requiredNotificationField(path string, object map[string]any, field string) (any, error) {
	value, present := object[field]
	if !present {
		return nil, fmt.Errorf("%s.%s is required", path, field)
	}
	return value, nil
}

func validateTopologyID(path string, raw any) error {
	value, ok := raw.(string)
	if !ok || !topologyIDPattern.MatchString(value) {
		return fmt.Errorf("%s is not a valid identifier", path)
	}
	return nil
}

func rejectUnknownFields(path string, object map[string]any, allowed map[string]struct{}) error {
	for field := range object {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("%s.%s is unknown", path, field)
		}
	}
	return nil
}
