// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"fmt"
	"slices"
)

type compiledFallbackClassification struct {
	registration   string
	classification string
	exactFamilies  []string
	embedded       *compiledEmbeddedRegistration
}

func (c *semanticCompiler) compileFallbackClassifications() error {
	for _, signalID := range sortedMapKeys(c.program.signals) {
		signal := c.program.signals[signalID]
		for _, registrationKey := range signal.registrations {
			registration := c.program.registrations[registrationKey]
			if registration.prometheus.Type != "untyped" {
				continue
			}
			language, err := c.registrationLanguage(registration)
			if err != nil {
				return err
			}
			classification := compiledFallbackClassification{
				registration:   registrationKey,
				classification: registration.prometheus.Classification,
				exactFamilies:  slices.Clone(language.exact),
				embedded:       language.embedded,
			}
			if previous, ok := c.program.fallbacks[registrationKey]; ok {
				if previous.classification != classification.classification {
					return fmt.Errorf("untyped registration %q has conflicting classifications %q and %q",
						registrationKey, previous.classification, classification.classification)
				}
				continue
			}
			c.program.fallbacks[registrationKey] = classification
		}
	}
	return nil
}
