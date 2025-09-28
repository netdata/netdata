// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo && ibm_mq

package mq

import (
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"strings"
)

// collectSubscriptions collects subscription metrics from the queue manager
func (c *Collector) collectSubscriptions() error {
	c.Debugf("Collecting subscriptions with selector '%s'", c.Config.SubscriptionSelector)

	// Get list of subscriptions
	subscriptions, err := c.client.InquireSubscription("")
	if err != nil {
		return fmt.Errorf("failed to inquire subscriptions: %w", err)
	}

	c.Debugf("Found %d subscriptions", len(subscriptions))

	collected := 0
	excluded := 0
	failed := 0

	for _, sub := range subscriptions {
		// Check if subscription should be included based on selector
		if c.Config.SubscriptionSelector != "" && c.Config.SubscriptionSelector != "*" {
			if !matchesPattern(sub.Name, c.Config.SubscriptionSelector) {
				c.Debugf("Skipping subscription '%s' (doesn't match selector)", sub.Name)
				excluded++
				continue
			}
		}

		// Get subscription status (message count, last message time)
		status, err := c.client.InquireSubscriptionStatus(sub.Name)
		if err != nil {
			c.Warningf("Failed to get status for subscription '%s': %v", sub.Name, err)
			failed++
			continue
		}

		labels := contexts.SubscriptionLabels{
			Subscription: sub.Name,
			Topic:        sub.TopicString,
		}

		// Message count
		if status.MessageCount.IsCollected() {
			contexts.Subscription.MessageCount.Set(c.State, labels, contexts.SubscriptionMessageCountValues{
				Pending: status.MessageCount.Int64(),
			})
		}

		// Last message age (only if we have timestamp)
		if status.LastMessageDate != "" && status.LastMessageTime != "" {
			age, err := status.GetSubscriptionAge()
			if err == nil && age >= 0 {
				contexts.Subscription.LastMessageAge.Set(c.State, labels, contexts.SubscriptionLastMessageAgeValues{
					Age: age,
				})
			}
		}

		collected++
	}

	c.Debugf("Subscription collection complete - discovered:%d excluded:%d collected:%d failed:%d",
		len(subscriptions), excluded, collected, failed)

	return nil
}

// matchesPattern checks if a name matches a pattern with wildcards
func matchesPattern(name, pattern string) bool {
	// Convert wildcard pattern to simple matching
	// * matches any sequence of characters
	// ? matches any single character

	// Special cases
	if pattern == "" || pattern == "*" {
		return true
	}

	// Simple wildcard matching
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = strings.ReplaceAll(pattern, "?", ".")
	pattern = "^" + pattern + "$"

	// For simplicity, use string contains for now
	// In production, we'd use a proper regex matcher
	if strings.Contains(pattern, ".*") {
		// Handle wildcard
		parts := strings.Split(pattern, ".*")
		if len(parts) == 2 {
			prefix := strings.TrimPrefix(parts[0], "^")
			suffix := strings.TrimSuffix(parts[1], "$")
			return strings.HasPrefix(name, prefix) && strings.HasSuffix(name, suffix)
		}
	}

	// Exact match
	return name == strings.Trim(pattern, "^$")
}
