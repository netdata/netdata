// SPDX-License-Identifier: GPL-3.0-or-later

package legacyconfig

import (
	"errors"
	"strings"
)

const (
	maxValueSize = 1 << 20
	maxStateSize = 4 << 20
)

// Settings contains literal evaluated values and unexecuted function source.
// Recipients are still strings: delivery routing, provider validation and function
// execution are separate contracts and are not performed by this reader.
type Settings struct {
	Variables  map[string]string
	Recipients map[string]map[string]string
	Functions  map[string]string
}

// Evaluate applies programs in order to a copy of explicitly supplied variables.
// Undefined references expand to empty strings; references are expanded only once,
// when assigned. The process environment is never consulted.
func Evaluate(initial map[string]string, programs ...*Program) (Settings, error) {
	state := Settings{
		Variables: make(map[string]string), Recipients: make(map[string]map[string]string), Functions: make(map[string]string),
	}
	size := 0
	set := func(values map[string]string, name, value string) error {
		if len(value) > maxValueSize {
			return errors.New("expanded value exceeds the 1 MiB limit")
		}
		previous, found := values[name]
		next := size + len(value) - len(previous)
		if !found {
			next += len(name)
		}
		if next > maxStateSize {
			return errors.New("evaluated configuration exceeds the 4 MiB limit")
		}
		size = next
		values[name] = value
		return nil
	}
	for name, value := range initial {
		if recipientMap(name) {
			return Settings{}, errors.New("initial scalar variables cannot name recipient maps")
		}
		if err := set(state.Variables, name, value); err != nil {
			return Settings{}, err
		}
	}
	for _, program := range programs {
		if program == nil {
			return Settings{}, errors.New("missing parsed legacy configuration")
		}
		for _, op := range program.instructions {
			if op.function != "" {
				if err := set(state.Functions, op.name, op.function); err != nil {
					return Settings{}, op.position.error(err.Error())
				}
				continue
			}
			if recipientMap(op.name) {
				entries, exists := state.Recipients[op.name]
				if !exists {
					size += len(op.name)
				}
				if op.reset {
					for key, value := range entries {
						size -= len(key) + len(value)
					}
				}
				if !exists || op.reset {
					entries = make(map[string]string)
					state.Recipients[op.name] = entries
				}
				if size > maxStateSize {
					return Settings{}, op.position.error("evaluated configuration exceeds the 4 MiB limit")
				}
				if op.key == "" {
					continue
				}
				value, err := op.value.evaluate(state.Variables)
				if err != nil {
					return Settings{}, op.position.error(err.Error())
				}
				if err := set(entries, op.key, value); err != nil {
					return Settings{}, op.position.error(err.Error())
				}
				continue
			}
			value, err := op.value.evaluate(state.Variables)
			if err != nil {
				return Settings{}, op.position.error(err.Error())
			}
			if err := set(state.Variables, op.name, value); err != nil {
				return Settings{}, op.position.error(err.Error())
			}
		}
	}
	return state, nil
}

func (e expression) evaluate(variables map[string]string) (string, error) {
	var result strings.Builder
	for _, part := range e {
		value := part.literal
		if part.variable != "" {
			value = variables[part.variable]
		}
		if len(value) > maxValueSize-result.Len() {
			return "", errors.New("expanded value exceeds the 1 MiB limit")
		}
		result.WriteString(value)
	}
	return result.String(), nil
}
