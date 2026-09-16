// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

const commandDefaultPath = "/usr/local/bin:/usr/bin:/bin"

func (dst Destination) validateCommand() error {
	allowed := Destination{Type: dst.Type, Executable: dst.Executable, Env: dst.Env}
	if dst.Type == "command" {
		allowed.Args = dst.Args
	} else {
		allowed.To = dst.To
	}
	if !reflect.DeepEqual(dst, allowed) {
		return errors.New("command destination contains fields for another provider")
	}
	if !filepath.IsAbs(dst.Executable) || strings.ContainsAny(dst.Executable, "\x00\r\n") || strings.Contains(dst.Executable, "${") {
		return errors.New("executable must be a literal absolute path without NUL or line breaks")
	}
	for _, arg := range dst.Args {
		if strings.ContainsRune(arg, 0) {
			return errors.New("command args must not contain NUL")
		}
	}
	for name, value := range dst.Env {
		if !validEnvironmentName(name) {
			return errors.New("command env names must use letters, digits and underscores, without a leading digit")
		}
		if _, err := secretReference(value); err != nil {
			return fmt.Errorf("command env: %w", err)
		}
		if strings.ContainsRune(value, 0) {
			return errors.New("command env values must not contain NUL")
		}
	}
	if dst.Type == "smstools3" {
		return validatePhoneNumber(dst.To, "smstools3", "to")
	}
	return nil
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && !(r >= 'A' && r <= 'Z') && !(r >= 'a' && r <= 'z') && !(i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func commandEnvironment(ctx context.Context, configured map[string]string) ([]string, error) {
	values := map[string]string{"PATH": commandDefaultPath}
	for name, raw := range configured {
		value, err := resolveSecret(ctx, raw)
		if err != nil {
			return nil, fmt.Errorf("command env: %w", err)
		}
		if strings.ContainsRune(value, 0) {
			return nil, errors.New("command env values must not contain NUL")
		}
		values[name] = value
	}
	env := make([]string, 0, len(values))
	for name, value := range values {
		env = append(env, name+"="+value)
	}
	sort.Strings(env)
	return env, nil
}

func sendCommand(ctx context.Context, processes *commandProcesses, dst Destination, event Event) error {
	env, err := commandEnvironment(ctx, dst.Env)
	if err != nil {
		return err
	}
	args := dst.Args
	var input []byte
	if dst.Type == "smstools3" {
		args = []string{dst.To, renderSMSTools3(event)}
	} else {
		input, err = json.Marshal(event)
		if err != nil {
			return errors.New("could not encode command event")
		}
		input = append(input, '\n')
	}
	return processes.run(ctx, dst.Executable, args, env, bytes.NewReader(input))
}

func renderSMSTools3(event Event) string {
	status := "needs attention"
	switch event.Status {
	case "CRITICAL":
		status = "is critical"
	case "CLEAR":
		status = "recovered"
	}
	text := event.Node + " " + status + ": "
	if event.Chart != "" {
		text += event.Chart + ", "
	}
	text += strings.ReplaceAll(event.Summary, "_", " ")
	if event.Status != "CLEAR" && event.Value != nil {
		text += " = " + strconv.FormatFloat(*event.Value, 'g', -1, 64)
		if event.Units != "" {
			text += " " + event.Units
		}
	}
	count := 0
	for index := range text {
		if count == 160 {
			return text[:index]
		}
		count++
	}
	return text
}
