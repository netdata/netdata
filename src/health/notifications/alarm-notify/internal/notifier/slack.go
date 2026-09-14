// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
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

func sendSlack(ctx context.Context, dst Destination, event Event, timeout time.Duration) error {
	message, err := renderSlack(event)
	if err != nil {
		return err
	}
	return postJSON(ctx, dst, message, timeout)
}

func renderSlack(event Event) (slackMessage, error) {
	color := map[string]string{"WARNING": "warning", "CRITICAL": "danger", "CLEAR": "good"}[event.Status]
	status := event.Status
	if event.PreviousStatus != "" && event.PreviousStatus != event.Status {
		status = event.PreviousStatus + " → " + event.Status
	}
	summary, err := slackPlainText(event.Status+": "+event.Summary, slackSectionLimit)
	if err != nil {
		return slackMessage{}, err
	}
	var blocks []slackBlock
	values := []string{"Node\n" + event.Node, "Alert\n" + event.Alert, "Status\n" + status}
	if event.Chart != "" {
		values = append(values, "Chart\n"+event.Chart)
	}
	if event.Context != "" {
		values = append(values, "Context\n"+event.Context)
	}
	for _, value := range []struct {
		label string
		value *float64
	}{{"Value", event.Value}, {"Previous value", event.PreviousValue}} {
		if value.value != nil {
			formatted := strconv.FormatFloat(*value.value, 'g', -1, 64)
			if event.Units != "" {
				formatted += " " + event.Units
			}
			values = append(values, value.label+"\n"+formatted)
		}
	}
	values = append(values, "Time\n"+event.Timestamp.Format(time.RFC3339))
	fields := make([]slackText, 0, len(values)) // At most eight fields, below Slack's limit of ten.
	fallback := []string{summary.Text}
	for _, value := range values {
		field, err := slackPlainText(value, slackFieldLimit)
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
