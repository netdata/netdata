// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version      int                    `yaml:"version"`
	Destinations map[string]Destination `yaml:"destinations"`
	Routing      Routing                `yaml:"routing,omitempty"`
}

type Routing struct {
	Roles   map[string][]string `yaml:"roles,omitempty"`
	Default []string            `yaml:"default,omitempty"`
}

type Destination struct {
	Type            string         `yaml:"type"`
	URL             string         `yaml:"url,omitempty"`
	BearerToken     string         `yaml:"bearer_token,omitempty"`
	BotToken        string         `yaml:"bot_token,omitempty"`
	AppToken        string         `yaml:"app_token,omitempty"`
	UserKey         string         `yaml:"user_key,omitempty"`
	AccessToken     string         `yaml:"access_token,omitempty"`
	Email           string         `yaml:"email,omitempty"`
	ChannelTag      string         `yaml:"channel_tag,omitempty"`
	SourceDeviceID  string         `yaml:"source_device_id,omitempty"`
	AccountSID      string         `yaml:"account_sid,omitempty"`
	AuthToken       string         `yaml:"auth_token,omitempty"`
	From            string         `yaml:"from,omitempty"`
	To              string         `yaml:"to,omitempty"`
	AccessKey       string         `yaml:"access_key,omitempty"`
	Originator      string         `yaml:"originator,omitempty"`
	Recipient       string         `yaml:"recipient,omitempty"`
	ChatID          string         `yaml:"chat_id,omitempty"`
	MessageThreadID *configInteger `yaml:"message_thread_id,omitempty"`
	APIURL          string         `yaml:"api_url,omitempty"`
	RetriesOnLimit  *configInteger `yaml:"retries_on_limit,omitempty"`
}

// YAML normally truncates floats assigned to integers, which could select the wrong topic.
type configInteger int64

func (value *configInteger) UnmarshalYAML(node *yaml.Node) error {
	if node.Tag != "!!int" {
		return errors.New("expected an integer")
	}
	var integer int64
	if err := node.Decode(&integer); err != nil {
		return err
	}
	*value = configInteger(integer)
	return nil
}

func readConfig(r io.Reader) (Config, error) {
	var cfg Config
	decoder := yaml.NewDecoder(r)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		// Decoder errors can quote credential-bearing scalar values and keys.
		return Config{}, errors.New("invalid YAML configuration: check syntax, field names, and types")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Config{}, errors.New("configuration must contain exactly one YAML document")
	}
	if cfg.Version != 1 {
		return Config{}, errors.New("configuration version must be 1")
	}
	if len(cfg.Destinations) == 0 {
		return Config{}, errors.New("configuration requires at least one destination")
	}
	for name, dst := range cfg.Destinations {
		if strings.TrimSpace(name) == "" {
			return Config{}, errors.New("destination name must not be empty")
		}
		if err := dst.validate(); err != nil {
			return Config{}, err
		}
	}
	if err := cfg.validateRouting(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (dst Destination) validate() error {
	if dst.Type == "messagebird" {
		return dst.validateMessageBird()
	}
	if dst.AccessKey != "" || dst.Originator != "" || dst.Recipient != "" {
		return errors.New("access_key, originator and recipient require type: messagebird")
	}
	if dst.Type == "twilio" {
		return dst.validateTwilio()
	}
	if dst.AccountSID != "" || dst.AuthToken != "" || dst.From != "" || dst.To != "" {
		return errors.New("account_sid, auth_token, from and to require type: twilio")
	}
	if dst.Type == "pushbullet" {
		return dst.validatePushbullet()
	}
	if dst.AccessToken != "" || dst.Email != "" || dst.ChannelTag != "" || dst.SourceDeviceID != "" {
		return errors.New("access_token, email, channel_tag and source_device_id require type: pushbullet")
	}
	if dst.Type == "pushover" {
		return dst.validatePushover()
	}
	if dst.AppToken != "" || dst.UserKey != "" {
		return errors.New("app_token and user_key require type: pushover")
	}
	if dst.Type == "telegram" {
		return dst.validateTelegram()
	}
	if dst.Type != "webhook" && dst.Type != "slack" && dst.Type != "discord" {
		return errors.New(
			"destination.type must be webhook, slack, discord, telegram, pushover, pushbullet, twilio or messagebird; other providers are not implemented yet",
		)
	}
	if dst.BotToken != "" || dst.ChatID != "" || dst.MessageThreadID != nil || dst.APIURL != "" ||
		dst.RetriesOnLimit != nil {
		return errors.New(
			"bot_token, chat_id, message_thread_id and retries_on_limit require type: telegram; api_url requires telegram, pushover, pushbullet, twilio or messagebird",
		)
	}
	if dst.Type != "webhook" && dst.BearerToken != "" {
		return fmt.Errorf("%s destinations authenticate through their URL; bearer_token is not supported", dst.Type)
	}
	reference, err := secretReference(dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if !reference {
		if err := validateURL(dst.URL); err != nil {
			return err
		}
		if dst.Type == "discord" {
			if _, err := discordEndpoint(dst.URL); err != nil {
				return err
			}
		}
	}
	if _, err := secretReference(dst.BearerToken); err != nil {
		return fmt.Errorf("destination.bearer_token: %w", err)
	}
	if strings.ContainsAny(dst.BearerToken, "\r\n") {
		return errors.New("destination.bearer_token must not contain line breaks")
	}
	return nil
}

func validateToken(value, provider, field string) error {
	if value == "" || strings.Contains(value, "${") ||
		strings.IndexFunc(value, func(r rune) bool { return r < 33 || r > 126 }) != -1 {
		return fmt.Errorf("%s %s must be nonempty printable ASCII without whitespace", provider, field)
	}
	return nil
}

// Secret references occupy the whole value; configuration validation performs no resolution.
func secretReference(value string) (bool, error) {
	if !strings.Contains(value, "${") {
		return false, nil
	}
	scheme, operand, ok := strings.Cut(value, ":")
	if !ok || !strings.HasSuffix(operand, "}") {
		return false, errors.New("expected a whole env or file secret reference")
	}
	operand = strings.TrimSuffix(operand, "}")
	if strings.TrimSpace(operand) == "" || strings.ContainsAny(operand, "{}") {
		return false, errors.New("secret reference requires a nonempty operand without braces")
	}
	switch scheme {
	case "${env":
		return true, nil
	case "${file":
		if filepath.IsAbs(operand) {
			return true, nil
		}
		return false, errors.New("file secret reference requires an absolute path")
	default:
		return false, errors.New("only whole env and file secret references are supported")
	}
}

func validateURL(value string) error {
	if !validHTTPURL(value, false) {
		return errors.New("destination.url must be an absolute HTTP(S) URL without user information or fragment")
	}
	return nil
}

func validHTTPURL(value string, allowFragment bool) bool {
	u, err := url.Parse(value)
	return err == nil && u.Hostname() != "" && u.Opaque == "" && u.User == nil &&
		(allowFragment || u.Fragment == "") && (u.Scheme == "http" || u.Scheme == "https")
}

func validateAPIBase(endpoint, provider, officialHost string) error {
	if endpoint == "" {
		return nil // The provider uses its official HTTPS endpoint.
	}
	// Even an empty fragment would capture the method appended to this base.
	if !validHTTPURL(endpoint, false) || strings.Contains(endpoint, "#") {
		return fmt.Errorf(
			"%s api_url must be an absolute HTTP(S) base URL without user information or fragment",
			provider,
		)
	}
	u, _ := url.Parse(endpoint)
	if u.RawQuery != "" || u.ForceQuery {
		return fmt.Errorf("%s api_url must not contain a query", provider)
	}
	if strings.EqualFold(strings.TrimSuffix(u.Hostname(), "."), officialHost) && u.Scheme != "https" {
		return fmt.Errorf("the official %s API requires HTTPS", provider)
	}
	return nil
}
