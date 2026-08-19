// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import "fmt"

type compiledStateEncoding struct {
	id           string
	definition   StateEncoding
	availability compiledEnvironmentCondition
	occurrences  []string
}

func (c *semanticCompiler) compileStateEncodings() error {
	bySource := make(map[string][]*compiledStateEncoding)
	for _, id := range sortedMapKeys(c.input.Contract.Source.StateEncodings) {
		definition := c.input.Contract.Source.StateEncodings[id]
		availability, err := c.resolveCondition("state_encodings."+id+".when", definition.When)
		if err != nil {
			return err
		}
		compiled := &compiledStateEncoding{
			id:           id,
			definition:   definition,
			availability: availability,
		}
		signal := c.program.signals[definition.Signal]
		for _, occurrenceKey := range signal.occurrences {
			occurrence := c.program.occurrences[occurrenceKey]
			if occurrence.component != definition.Component {
				continue
			}
			active := occurrence.availability.and(availability, c.environment.axes)
			if len(active.clauses) != 0 {
				compiled.occurrences = append(compiled.occurrences, occurrenceKey)
			}
		}
		if len(compiled.occurrences) == 0 {
			return fmt.Errorf("state encoding %q is inactive", id)
		}
		key := definition.Signal + "#" + definition.Component + "#" + definition.Label
		for _, previous := range bySource[key] {
			if previous.availability.overlaps(availability, c.environment.axes) {
				return fmt.Errorf("state encodings %q and %q overlap for %s",
					previous.id, id, key)
			}
		}
		bySource[key] = append(bySource[key], compiled)
		c.program.stateEncodings[id] = compiled
	}
	return nil
}

func (c *semanticCompiler) validateViewStateEncodings(contextID string, view *compiledView) error {
	for _, inputID := range sortedMapKeys(view.inputs) {
		input := view.inputs[inputID]
		for _, source := range input.occurrences {
			if source.component.source.Unit.Quantity != "state" {
				continue
			}
			relevantLabels := make(map[string]struct{})
			for label := range view.labels.Dimensions {
				if _, ok := source.occurrence.labels[label]; ok {
					relevantLabels[label] = struct{}{}
				}
			}
			if len(relevantLabels) == 0 {
				continue
			}
			covering := make([]compiledEnvironmentCondition, 0)
			for _, encoding := range source.program.stateEncodings {
				definition := encoding.definition
				if definition.Signal == source.occurrence.signal &&
					definition.Component == source.occurrence.component {
					if _, ok := relevantLabels[definition.Label]; ok {
						covering = append(covering, encoding.availability)
					}
				}
			}
			if !source.occurrence.availability.coveredBy(
				source.program.environment.axes,
				covering...,
			) {
				return fmt.Errorf("view %q input %q state component %s/%s lacks complete state-encoding coverage",
					contextID, inputID, source.occurrence.signal, source.occurrence.component)
			}
		}
	}
	return nil
}
