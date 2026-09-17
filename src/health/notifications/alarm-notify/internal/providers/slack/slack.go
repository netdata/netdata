// SPDX-License-Identifier: GPL-3.0-or-later

package slack

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	notifyevent "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/event"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/httpclient"
	notifymsg "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/message"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

// Slack's section text/field limits are documented in its Block Kit reference.
// https://docs.slack.dev/reference/block-kit/blocks/section-block/
const (
	slackSectionLimit = 3000
	slackFieldLimit   = 2000
)

var (
	slackEscaper     = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	slackLinkEscaper = strings.NewReplacer("<", "%3C", ">", "%3E", "|", "%7C", " ", "%20", "`", "%60")
)

type slackMessage struct {
	Text        string            `json:"text"`
	Mrkdwn      bool              `json:"mrkdwn"`
	Parse       string            `json:"parse"`
	UnfurlLinks bool              `json:"unfurl_links"`
	UnfurlMedia bool              `json:"unfurl_media"`
	Blocks      []slackBlock      `json:"blocks"`
	Attachments []slackAttachment `json:"attachments"`
}

type slackAttachment struct {
	Color  string       `json:"color"`
	Blocks []slackBlock `json:"blocks"`
}

type slackBlock struct {
	Type   string      `json:"type"`
	Text   *slackText  `json:"text,omitempty"`
	Fields []slackText `json:"fields,omitempty"`
}

type slackText struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Verbatim bool   `json:"verbatim,omitempty"`
}

func sendSlack(ctx context.Context, dst Config, event notifyevent.Event, client *http.Client) error {
	message, err := renderSlack(event)
	if err != nil {
		return err
	}
	return postJSON(ctx, client, dst, message)
}

func renderSlack(event notifyevent.Event) (slackMessage, error) {
	color := map[string]string{"WARNING": "warning", "CRITICAL": "danger", "CLEAR": "good"}[event.Status]
	summary, err := slackPlainText(event.Status+": "+event.Summary, slackSectionLimit)
	if err != nil {
		return slackMessage{}, err
	}
	var blocks []slackBlock
	values := append(notifymsg.Fields(event), notifymsg.Field{Name: "Time", Value: event.Timestamp.Format(time.RFC3339)})
	fields := make([]slackText, 0, len(values)) // At most eight fields, below Slack's limit of ten.
	fallback := []string{summary.Text}
	for _, value := range values {
		field, err := slackPlainText(value.Name+"\n"+value.Value, slackFieldLimit)
		if err != nil {
			return slackMessage{}, err
		}
		fields = append(fields, field)
		fallback = append(fallback, field.Text)
	}
	blocks = append(blocks, slackBlock{Type: "section", Fields: fields})
	if event.Info != "" {
		info, err := slackPlainText(event.Info, slackSectionLimit)
		if err != nil {
			return slackMessage{}, err
		}
		blocks = append(blocks, slackBlock{Type: "section", Text: &info})
		fallback = append(fallback, info.Text)
	}
	if event.URL != "" {
		link, err := slackLinkText(event.URL)
		if err != nil {
			return slackMessage{}, err
		}
		blocks = append(blocks, slackBlock{Type: "section", Text: &link})
		fallback = append(fallback, "View alert\n"+slackEscaper.Replace(event.URL))
	}
	return slackMessage{
		Text: strings.Join(fallback, "\n\n"), Parse: "none",
		// Root blocks make Text a fallback instead of duplicating the attachment as a visible body.
		Blocks:      []slackBlock{{Type: "section", Text: &summary}},
		Attachments: []slackAttachment{{Color: color, Blocks: blocks}},
	}, nil
}

func slackLinkText(value string) (slackText, error) {
	// An explicit link needs no interaction server. Encode delimiters before adding Slack markup.
	value = "<" + slackEscaper.Replace(slackLinkEscaper.Replace(value)) + "|View alert>"
	if utf8.RuneCountInString(value) > slackSectionLimit {
		return slackText{}, errors.New("slack navigation link exceeds the 3000-character Block Kit limit")
	}
	return slackText{Type: "mrkdwn", Text: value, Verbatim: true}, nil
}

func slackPlainText(value string, limit int) (slackText, error) {
	// Escape Slack control characters; JSON escaping alone does not prevent mentions.
	value = slackEscaper.Replace(value)
	if utf8.RuneCountInString(value) > limit {
		return slackText{}, fmt.Errorf("slack text exceeds the %d-character Block Kit limit", limit)
	}
	return slackText{Type: "plain_text", Text: value}, nil
}

func (dst Config) validateSlack() error {
	reference, err := secret.IsReference(dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if !reference {
		if err := field.URL(dst.URL); err != nil {
			return err
		}
	}

	return nil
}

func postJSON(ctx context.Context, client *http.Client, dst Config, message any) error {
	endpoint, err := secret.Resolve(ctx, dst.URL)
	if err != nil {
		return fmt.Errorf("destination.url: %w", err)
	}
	if err := field.URL(endpoint); err != nil {
		return err
	}
	headers := http.Header{}
	response, err := httpclient.PostJSON(ctx, client, "slack", endpoint, headers, message)
	if err != nil {
		return err
	}
	// These providers acknowledge delivery through HTTP status, not the response body.
	// Close without buffering or draining an arbitrary remote body.
	defer response.Body.Close()
	accepted := response.StatusCode == http.StatusOK
	if !accepted {
		return fmt.Errorf("slack returned HTTP %d", response.StatusCode)
	}
	return nil
}

type Config struct {
	URL string `yaml:"url,omitempty"`
}

type Sender struct {
	config Config
	client *http.Client
}

func New(cfg Config, client *http.Client) (*Sender, error) {
	if err := cfg.validateSlack(); err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errors.New("slack HTTP client is required")
	}
	return &Sender{config: cfg, client: client}, nil
}
func (s *Sender) Send(ctx context.Context, event notifyevent.Event) error {
	return sendSlack(ctx, s.config, event, s.client)
}
