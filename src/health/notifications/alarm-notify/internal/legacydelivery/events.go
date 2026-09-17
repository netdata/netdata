// SPDX-License-Identifier: GPL-3.0-or-later

package legacydelivery

import (
	"errors"
	"net/url"
	"strings"
	"unicode"

	configfield "github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/config/field"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/notifier"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/dynatrace"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/ilert"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/providers/opsgenie"
	"github.com/netdata/netdata/src/health/notifications/alarm-notify/internal/secret"
)

func buildIlert(b *builder, _ []string) ([]notifier.Sender, error) {
	return one(ilert.New(ilert.Config{
		Secrets: secret.LiteralInput, IntegrationKey: b.values["ILERT_INTEGRATION_KEY"], APIURL: b.values["ILERT_API_URL"],
	}, b.client))
}

func buildOpsgenie(b *builder, _ []string) ([]notifier.Sender, error) {
	return one(opsgenie.New(opsgenie.Config{
		Secrets: secret.LiteralInput, APIKey: b.values["OPSGENIE_API_KEY"], APIURL: b.values["OPSGENIE_API_URL"],
	}, b.client))
}

func buildDynatrace(b *builder, _ []string) ([]notifier.Sender, error) {
	server, space, tag := b.values["DYNATRACE_SERVER"], b.values["DYNATRACE_SPACE"], b.values["DYNATRACE_TAG_VALUE"]
	// Validate before appending, so a query or fragment cannot absorb the environment path.
	if err := configfield.APIBase(server, "dynatrace DYNATRACE_SERVER", ""); err != nil {
		return nil, err
	}
	if space == "." || space == ".." || strings.ContainsAny(space, "/\\%") || strings.IndexFunc(space, unicode.IsControl) >= 0 {
		return nil, errors.New("DYNATRACE_SPACE must be a single environment path segment without dot segments, separators, percent encodings or control characters")
	}
	// Only emit documented literal tag syntax. Do not reinterpret a manual tag as
	// a context/key/value expression or guess escaping for the selector's delimiters.
	if strings.TrimSpace(tag) != tag || strings.HasPrefix(tag, "[") || strings.ContainsAny(tag, "\"\\~") || strings.IndexFunc(tag, unicode.IsControl) >= 0 {
		return nil, errors.New("DYNATRACE_TAG_VALUE cannot be mapped literally with boundary whitespace, a leading context bracket, quotes, backslashes, tildes or control characters; use a native dynatrace entity_selector")
	}
	selector := `type(HOST),tag("` + strings.ReplaceAll(tag, ":", `\:`) + `")`
	return one(dynatrace.New(dynatrace.Config{
		Secrets: secret.LiteralInput, APIURL: strings.TrimSuffix(server, "/") + "/e/" + url.PathEscape(space),
		APIToken: b.values["DYNATRACE_TOKEN"], EntitySelector: selector,
		EventType: b.values["DYNATRACE_EVENT"], Source: b.values["DYNATRACE_ANNOTATION_TYPE"],
	}, b.client))
}
