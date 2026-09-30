// SPDX-License-Identifier: GPL-3.0-or-later

package native

import "errors"

// initDefinition loads a generic job's manifest or direct command. Registered
// jobs keep their startup definition, even if package files change.
func (c *Collector) initDefinition() error {
	if c.registered {
		if c.Manifest != "" || c.Command != nil || c.Mode != "" {
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
		definition, err := newPackageDefinition(packageSpec{
			Version: "v1",
			Mode:    c.Mode,
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
	if c.Mode != "" {
		return errors.New("mode is only supported with a direct command")
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
