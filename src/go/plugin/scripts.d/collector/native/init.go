// SPDX-License-Identifier: GPL-3.0-or-later

package native

import "errors"

// initDefinition loads a generic job's manifest. Registered package jobs keep
// the definition validated at plugin startup, even if package files change.
func (c *Collector) initDefinition() error {
	if c.registered {
		if c.Manifest != "" {
			return errors.New("registered packages cannot override manifest")
		}
		return nil
	}
	definition, err := loadManifest(c.Manifest, c.validateExecutable)
	if err != nil {
		return err
	}
	if len(definition.Functions) > 0 {
		return errors.New("packages with Functions must be registered in scripts.d.packages.yaml")
	}
	c.definition = definition
	return nil
}
