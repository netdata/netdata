// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

// TopicInfo contains information about an MQ topic
type TopicInfo struct {
	Name        string // Topic object name (e.g., "TOPIC.SALES")
	TopicString string // Topic string for pub/sub (e.g., "sales/orders")
}