// SPDX-License-Identifier: GPL-3.0-or-later

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
)

// Keep occurrence tracking in the input layer, separate from the decoded facts.
type producerContextInput struct {
	value notifier.ProducerContext
	seen  bool
}

func (p *producerContextInput) UnmarshalJSON(data []byte) error {
	if p.seen {
		return errors.New("duplicate producer_context")
	}
	p.seen = true

	var value notifier.ProducerContext
	decoder := json.NewDecoder(bytes.NewReader(data))
	// Outer decoder options do not propagate into UnmarshalJSON.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}

	// The typed decode permits only an object or null. Check member occurrences
	// separately: encoding/json otherwise merges repeated fields, even across null.
	decoder = json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		p.value = value
		return nil
	}
	var names []string
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		name := token.(string)
		for _, previous := range names {
			// Match encoding/json's Unicode case folding after JSON escape decoding.
			if strings.EqualFold(previous, name) {
				return errors.New("duplicate producer_context member")
			}
		}
		names = append(names, name)
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return err
	}
	p.value = value
	return nil
}
