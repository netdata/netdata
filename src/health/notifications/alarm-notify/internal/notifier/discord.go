// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
)

// https://docs.discord.com/developers/resources/message#embed-limits
const (
	discordTitleLimit       = 256
	discordDescriptionLimit = 4096
	discordFieldLimit       = 1024
	discordEmbedLimit       = 6000
)

var discordEscaper = strings.NewReplacer(
	`\`, `\\`, "*", `\*`, "_", `\_`, "~", `\~`, "`", "\\`", "|", `\|`,
	"[", `\[`, "]", `\]`, "(", `\(`, ")", `\)`, "<", `\<`, ">", `\>`,
	"#", `\#`, "-", `\-`, "+", `\+`, ".", `\.`,
)

type discordMessage struct {
	Username        string                 `json:"username"`
	Embeds          []discordEmbed         `json:"embeds"`
	AllowedMentions discordAllowedMentions `json:"allowed_mentions"`
}

type discordAllowedMentions struct {
	Parse []string `json:"parse"`
}

type discordEmbed struct {
	Title       string              `json:"title"`
	Description string              `json:"description,omitempty"`
	URL         string              `json:"url,omitempty"`
	Color       int                 `json:"color"`
	Timestamp   string              `json:"timestamp"`
	Fields      []discordEmbedField `json:"fields"`
}

type discordEmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

func sendDiscord(ctx context.Context, dst Destination, event notifyevent.Event, timeout time.Duration) error {
	message, err := renderDiscord(event)
	if err != nil {
		return err
	}
	return postJSON(ctx, dst, message, timeout)
}

func renderDiscord(event notifyevent.Event) (discordMessage, error) {
	title, err := discordText(event.Status+": "+event.Summary, discordTitleLimit)
	if err != nil {
		return discordMessage{}, err
	}
	description, err := discordText(event.Info, discordDescriptionLimit)
	if err != nil {
		return discordMessage{}, err
	}
	embed := discordEmbed{
		Title: title, Description: description, URL: event.URL,
		Color:     map[string]int{"WARNING": 0xFAA61A, "CRITICAL": 0xED4245, "CLEAR": 0x57F287}[event.Status],
		Timestamp: event.Timestamp.Format(time.RFC3339),
	}
	total := utf8.RuneCountInString(title) + utf8.RuneCountInString(description)
	for _, field := range notifymsg.Fields(event) {
		value, err := discordText(field.Value, discordFieldLimit)
		if err != nil {
			return discordMessage{}, err
		}
		if value == "" {
			continue // Discord does not accept empty fields; optional whitespace-only facts are absent.
		}
		// Names are fixed labels; at most seven fields, below Discord's limit of 25.
		embed.Fields = append(embed.Fields, discordEmbedField{Name: field.Name, Value: value, Inline: true})
		total += utf8.RuneCountInString(field.Name) + utf8.RuneCountInString(value)
	}
	if total > discordEmbedLimit {
		return discordMessage{}, errors.New("discord embed exceeds the 6000-character combined limit")
	}
	// Preserve Bash's compact host-derived sender name; full node identity remains in the embed.
	username := []rune("netdata on " + event.Node)
	if len(username) > 32 {
		username = append(username[:29], '.', '.', '.')
	}
	return discordMessage{
		Username: string(username), Embeds: []discordEmbed{embed},
		AllowedMentions: discordAllowedMentions{Parse: []string{}},
	}, nil
}

func discordText(value string, limit int) (string, error) {
	// Discord trims leading/trailing whitespace before applying its embed limits.
	value = discordEscaper.Replace(strings.TrimSpace(value))
	if utf8.RuneCountInString(value) > limit {
		return "", fmt.Errorf("discord text exceeds the %d-character embed limit", limit)
	}
	return value, nil
}

func discordEndpoint(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", errors.New("invalid discord webhook URL")
	}
	query, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", errors.New("invalid discord webhook URL query")
	}
	// Native execution defaults to unconfirmed delivery, which can hide unsaved messages.
	query.Set("wait", "true")
	u.RawQuery = query.Encode()
	return u.String(), nil
}
