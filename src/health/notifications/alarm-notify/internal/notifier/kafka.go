// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"errors"
	"fmt"
	"net/netip"
	"reflect"
)

func (dst Destination) validateKafka() error {
	if !reflect.DeepEqual(dst, Destination{Type: dst.Type, URL: dst.URL, SenderIP: dst.SenderIP}) {
		return errors.New("kafka destination contains fields for another provider")
	}
	address, err := netip.ParseAddr(dst.SenderIP)
	if err != nil || address.Zone() != "" {
		return errors.New("kafka sender_ip must be a literal IPv4 or IPv6 address without a zone")
	}
	reference, err := secretReference(dst.URL)
	if err != nil {
		return fmt.Errorf("kafka url: %w", err)
	}
	if !reference {
		return validateURL(dst.URL)
	}
	return nil
}

// Kafka uses the historical HTTP bridge document, not the Kafka wire protocol.
type kafkaMessage struct {
	HostIP           string   `json:"host_ip"`
	When             int64    `json:"when"`
	Name             string   `json:"name"`
	Chart            string   `json:"chart"`
	Status           string   `json:"status"`
	OldStatus        string   `json:"old_status"`
	Value            *float64 `json:"value"`
	OldValue         *float64 `json:"old_value"`
	Duration         *uint32  `json:"duration"`
	NonClearDuration *uint32  `json:"non_clear_duration"`
	Units            string   `json:"units"`
	Info             string   `json:"info"`
}

func renderKafka(dst Destination, event Event) kafkaMessage {
	return kafkaMessage{
		HostIP: dst.SenderIP, When: event.Timestamp.Unix(), Name: event.Alert, Chart: event.Chart,
		Status: event.Status, OldStatus: event.PreviousStatus, Value: event.Value, OldValue: event.PreviousValue,
		Duration: event.Duration, NonClearDuration: event.NonClearDuration, Units: event.Units, Info: event.Info,
	}
}
