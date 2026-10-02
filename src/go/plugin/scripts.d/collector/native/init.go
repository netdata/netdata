// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"errors"
	"fmt"
)

// initDefinition loads a generic job's manifest or direct command. Registered
// jobs keep their startup definition, even if package files change.
func (c *Collector) initDefinition() error {
	if err := c.Mode.Validate(); err != nil {
		return fmt.Errorf("mode %w", err)
	}
	mode := c.Mode.Normalized()
	if c.registered {
		if c.Manifest != "" || c.Command != nil || mode != modeAuto {
			return errors.New("registered packages cannot override manifest, command or mode")
		}
		return nil
	}
	if (c.Manifest == "") == (c.Command == nil) {
		return errors.New("requires exactly one manifest or command")
	}
	if c.Command != nil {
		command, err := validatedCommand(c.Command, c.validateExecutable)
		if err != nil {
			return err
		}
		if mode == modeAuto {
			mode = modeOneshot
		}
		definition, err := newPackageDefinition(packageSpec{
			Version: "v1",
			Mode:    string(mode),
		}, command)
		if err != nil {
			return err
		}
		if definition.templates, err = definition.chartTemplates(nil); err != nil {
			return err
		}
		c.definition = definition
		return nil
	}
	if mode != modeAuto {
		return errors.New("mode must be auto when using a manifest")
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
