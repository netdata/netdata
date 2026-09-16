// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/commandexec"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

const snsMessageLimit = 262144

var (
	snsARNPattern     = regexp.MustCompile(`^arn:(aws(?:-[a-z]+)*):sns:([a-z0-9]+(?:-[a-z0-9]+)+):[0-9]{12}:([A-Za-z0-9_-]{1,256}|endpoint/[A-Za-z0-9_-]+/[A-Za-z0-9_.-]{1,256}/[A-Za-z0-9-]+)$`)
	snsRolePattern    = regexp.MustCompile(`^arn:aws(?:-[a-z]+)*:iam::[0-9]{12}:role/(?:[\x21-\x7e]+/)?[A-Za-z0-9_+=,.@-]{1,64}$`)
	snsSessionPattern = regexp.MustCompile(`^[A-Za-z0-9_+=,.@-]{2,64}$`)
)

type snsPublish struct {
	TargetARN string `json:"TargetArn"`
	Subject   string `json:"Subject"`
	Message   string `json:"Message"`
}

func (dst Destination) validateAWSSNS() error {
	allowed := Destination{Type: dst.Type, Executable: dst.Executable, Env: dst.Env,
		TargetARN: dst.TargetARN, CredentialSource: dst.CredentialSource, MessageTemplate: dst.MessageTemplate}
	if !reflect.DeepEqual(dst, allowed) {
		return errors.New("awssns destination contains fields for another provider")
	}
	if err := (Destination{Type: "command", Executable: dst.Executable}).validateCommand(); err != nil {
		return fmt.Errorf("awssns: %w", err)
	}
	if !snsARNPattern.MatchString(dst.TargetARN) {
		return errors.New("awssns target_arn must be a literal standard SNS topic or platform endpoint ARN")
	}
	if err := validateSNSCredentials(dst.CredentialSource, dst.Env, true); err != nil {
		return err
	}
	_, err := renderSNSTemplate(dst.MessageTemplate, snsFields(notifyevent.Event{}))
	return err
}

func validateSNSCredentials(source string, env map[string]string, references bool) error {
	var required, optional []string
	switch source {
	case "static":
		required = []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}
		optional = []string{"AWS_SESSION_TOKEN"}
	case "web_identity":
		required = []string{"AWS_ROLE_ARN", "AWS_WEB_IDENTITY_TOKEN_FILE"}
		optional = []string{"AWS_ROLE_SESSION_NAME"}
	case "ecs":
		required = []string{"AWS_CONTAINER_CREDENTIALS_RELATIVE_URI"}
	case "imds":
	default:
		return errors.New("awssns credential_source must be static, web_identity, ecs or imds")
	}
	allowed := make(map[string]bool)
	for _, key := range required {
		allowed[key] = true
		if env[key] == "" {
			return errors.New("awssns env is missing a required credential_source variable")
		}
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key, value := range env {
		if !allowed[key] {
			return errors.New("awssns env contains a variable outside the selected credential_source")
		}
		if references {
			ref, err := secret.IsReference(value)
			if err != nil {
				return fmt.Errorf("awssns env: %w", err)
			}
			if ref {
				continue
			}
		}
		if value == "" || !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool {
			return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
		}) != -1 {
			return errors.New("awssns credential variables must be nonempty UTF-8 without controls")
		}
		switch key {
		case "AWS_ROLE_ARN":
			if !snsRolePattern.MatchString(value) {
				return errors.New("awssns AWS_ROLE_ARN must be an IAM role ARN")
			}
		case "AWS_ROLE_SESSION_NAME":
			if !snsSessionPattern.MatchString(value) {
				return errors.New("awssns AWS_ROLE_SESSION_NAME must be 2-64 valid session-name characters")
			}
		case "AWS_WEB_IDENTITY_TOKEN_FILE":
			if !filepath.IsAbs(value) {
				return errors.New("awssns AWS_WEB_IDENTITY_TOKEN_FILE must be an absolute path")
			}
		case "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI":
			// Botocore concatenates this value to the metadata origin; require a path, not an authority suffix.
			u, err := url.Parse("http://169.254.170.2" + value)
			if !strings.HasPrefix(value, "/") || strings.ContainsAny(value, "\\#") ||
				strings.IndexFunc(value, unicode.IsSpace) != -1 || err != nil || u.Host != "169.254.170.2" || u.User != nil {
				return errors.New("awssns ECS credentials URI must be a relative path on the fixed ECS metadata endpoint")
			}
		}
	}
	return nil
}

func snsFields(event notifyevent.Event) map[string]string {
	duration := func(seconds *uint32) string {
		if seconds == nil {
			return ""
		}
		return strconv.FormatUint(uint64(*seconds), 10)
	}
	value := func(number *float64) string {
		if number == nil {
			return ""
		}
		return strconv.FormatFloat(*number, 'g', -1, 64)
	}
	withUnits := func(number *float64) string {
		text := value(number)
		if text != "" && event.Units != "" {
			text += " " + event.Units
		}
		return text
	}
	status := "needs attention"
	switch event.Status {
	case "CRITICAL":
		status = "is critical"
	case "CLEAR":
		status = "recovered"
	}
	return map[string]string{
		"version": strconv.Itoa(event.Version), "incident_id": event.IncidentID,
		"timestamp": event.Timestamp.Format(time.RFC3339), "node": event.Node, "alert": event.Alert,
		"chart": event.Chart, "context": event.Context, "status": event.Status, "previous_status": event.PreviousStatus,
		"summary": event.Summary, "info": event.Info, "value": value(event.Value), "previous_value": value(event.PreviousValue),
		"units": event.Units, "url": event.URL, "status_message": status,
		"duration": duration(event.Duration), "non_clear_duration": duration(event.NonClearDuration),
		"value_string": withUnits(event.Value), "previous_value_string": withUnits(event.PreviousValue),
	}
}

// Substitute once: event text can contain template or CLI-looking syntax without being evaluated.
func renderSNSTemplate(template string, fields map[string]string) (string, error) {
	if !utf8.ValidString(template) || len(template) > snsMessageLimit {
		return "", errors.New("awssns message_template must be UTF-8 and at most 262144 bytes")
	}
	var result strings.Builder
	write := func(text string) bool {
		if len(text) > snsMessageLimit-result.Len() {
			return false
		}
		result.WriteString(text)
		return true
	}
	for template != "" {
		index := strings.Index(template, "{{")
		if index == -1 {
			if !write(template) {
				return "", errors.New("awssns message exceeds 262144 bytes")
			}
			break
		}
		if !write(template[:index]) {
			return "", errors.New("awssns message exceeds 262144 bytes")
		}
		template = template[index:]
		if strings.HasPrefix(template, "{{{{") {
			if !write("{{") {
				return "", errors.New("awssns message exceeds 262144 bytes")
			}
			template = template[4:]
			continue
		}
		end := strings.Index(template[2:], "}}")
		if end == -1 {
			return "", errors.New("awssns message_template contains an unclosed placeholder")
		}
		text, ok := fields[strings.TrimSpace(template[2:2+end])]
		if !ok {
			return "", errors.New("awssns message_template contains an unknown placeholder")
		}
		if !write(text) {
			return "", errors.New("awssns message exceeds 262144 bytes")
		}
		template = template[end+4:]
	}
	return result.String(), nil
}

func renderSNS(dst Destination, event notifyevent.Event) (snsPublish, error) {
	fields := snsFields(event)
	subject := event.Node + " " + fields["status_message"] + " - " + strings.ReplaceAll(event.Alert, "_", " ")
	if event.Chart != "" {
		subject += " - " + event.Chart
	}
	if !utf8.ValidString(subject) || utf8.RuneCountInString(subject) >= 100 || strings.IndexFunc(subject, func(r rune) bool {
		return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
	}) != -1 {
		return snsPublish{}, errors.New("awssns subject must be UTF-8, under 100 characters, without controls or line breaks")
	}
	message := event.Status + " on " + event.Node + " at " + fields["timestamp"] + ":"
	if event.Chart != "" {
		message += " " + event.Chart
	}
	if fields["value_string"] != "" {
		message += " " + fields["value_string"]
	}
	if dst.MessageTemplate != "" {
		var err error
		message, err = renderSNSTemplate(dst.MessageTemplate, fields)
		if err != nil {
			return snsPublish{}, err
		}
	}
	if message == "" || !utf8.ValidString(message) || len(message) > snsMessageLimit {
		return snsPublish{}, errors.New("awssns message must be nonempty UTF-8 and at most 262144 bytes")
	}
	return snsPublish{TargetARN: dst.TargetARN, Subject: subject, Message: message}, nil
}

func snsEnvironment(ctx context.Context, dst Destination, region string) ([]string, error) {
	credentials := make(map[string]string, len(dst.Env))
	for key, raw := range dst.Env {
		value, err := secret.Resolve(ctx, raw)
		if err != nil {
			return nil, fmt.Errorf("awssns env: %w", err)
		}
		credentials[key] = value
	}
	if err := validateSNSCredentials(dst.CredentialSource, credentials, false); err != nil {
		return nil, err
	}
	values := map[string]string{
		"PATH": commandDefaultPath, "AWS_CONFIG_FILE": os.DevNull, "AWS_SHARED_CREDENTIALS_FILE": os.DevNull,
		"BOTO_CONFIG": os.DevNull, "AWS_IGNORE_CONFIGURED_ENDPOINT_URLS": "true",
		"AWS_MAX_ATTEMPTS": "1", "AWS_RETRY_MODE": "standard", "AWS_PAGER": "", "AWS_CLI_AUTO_PROMPT": "off",
		"AWS_CLI_FILE_ENCODING": "UTF-8", "AWS_EC2_METADATA_DISABLED": "true",
		"AWS_DEFAULT_REGION": region, "AWS_REGION": region, "NO_PROXY": "*",
	}
	if dst.CredentialSource == "imds" {
		values["AWS_EC2_METADATA_DISABLED"] = "false"
	}
	for key, value := range credentials {
		values[key] = value
	}
	env := make([]string, 0, len(values))
	for key, value := range values {
		env = append(env, key+"="+value)
	}
	sort.Strings(env)
	return env, nil
}

func sendAWSSNS(ctx context.Context, processes *commandexec.Runner, dst Destination, event notifyevent.Event) error {
	request, err := renderSNS(dst, event)
	if err != nil {
		return err
	}
	region := strings.Split(dst.TargetARN, ":")[3] // Configuration has validated the ARN.
	env, err := snsEnvironment(ctx, dst, region)
	if err != nil {
		return err
	}
	data, err := json.Marshal(request)
	if err != nil {
		return errors.New("could not encode awssns message")
	}
	args := []string{"sns", "publish", "--region", region, "--cli-input-json", "file:///dev/stdin",
		"--no-cli-pager", "--no-cli-auto-prompt", "--output", "json"}
	return processes.RunWithPrivateHome(ctx, dst.Executable, args, env, bytes.NewReader(append(data, '\n')))
}
