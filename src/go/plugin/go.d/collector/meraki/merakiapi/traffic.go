// SPDX-License-Identifier: GPL-3.0-or-later

package merakiapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	switchUsageOperation = "getOrganizationSwitchPortsUsageHistoryByDeviceByInterval"
	packetOperation      = "getDeviceSwitchPortsStatusesPackets"
	uplinkUsageOperation = "getOrganizationApplianceUplinksUsageByNetwork"
)

// ListSwitchPortUsageHistory reads every page for one exact source window.
// The API calculates interval from t0/t1; callers validate returned intervals.
func (c *Client) ListSwitchPortUsageHistory(ctx context.Context, organizationID string, start, end time.Time) ([]SwitchPortUsageHistory, error) {
	fixed, err := windowQuery(start, end, 31*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid time window", switchUsageOperation)
	}
	target, err := c.trafficURL(switchUsageOperation, "organizations", organizationID, "/switch/ports/usage/history/byDevice/byInterval", fixed)
	if err != nil {
		return nil, err
	}
	q := target.Query()
	q.Set("perPage", "50")
	target.RawQuery = q.Encode()
	operationPath := target.EscapedPath()
	visited := make(map[string]bool)
	items := make([]SwitchPortUsageHistory, 0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, safeError(switchUsageOperation, "request canceled", err)
		}
		target, err = c.switchUsagePageURL(target, operationPath, fixed, end.Sub(start))
		if err != nil {
			return nil, fmt.Errorf("%s: invalid pagination target", switchUsageOperation)
		}
		key := net.JoinHostPort(strings.ToLower(target.Hostname()), effectivePort(target)) + target.EscapedPath() + "?" + target.RawQuery
		if visited[key] {
			return nil, fmt.Errorf("%s: cyclic pagination", switchUsageOperation)
		}
		visited[key] = true
		p, err := fetchDecodedPage(ctx, c, target, switchUsageOperation, decodeItemsEnvelope[SwitchPortUsageHistory])
		if err != nil {
			return nil, err
		}
		items = append(items, p.items...)
		next, found, err := p.next()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", switchUsageOperation, err)
		}
		if !found {
			return items, nil
		}
		target = next
	}
}

// GetSwitchPortPackets returns the provider's window packet rates, not counters.
func (c *Client) GetSwitchPortPackets(ctx context.Context, serial string, timespan time.Duration) ([]SwitchPortPacketStatus, error) {
	if timespan <= 0 || timespan > 24*time.Hour {
		return nil, fmt.Errorf("%s: invalid timespan", packetOperation)
	}
	q := url.Values{"timespan": {strconv.FormatFloat(timespan.Seconds(), 'f', -1, 64)}}
	target, err := c.trafficURL(packetOperation, "devices", serial, "/switch/ports/statuses/packets", q)
	if err != nil {
		return nil, err
	}
	p, err := fetchPage[SwitchPortPacketStatus](ctx, c, target, packetOperation)
	if err != nil {
		return nil, err
	}
	return p.items, nil
}

// GetApplianceUplinkUsage returns requested-window network/interface byte totals.
func (c *Client) GetApplianceUplinkUsage(ctx context.Context, organizationID string, start, end time.Time) ([]ApplianceUplinkUsage, error) {
	q, err := windowQuery(start, end, 14*24*time.Hour)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid time window", uplinkUsageOperation)
	}
	target, err := c.trafficURL(uplinkUsageOperation, "organizations", organizationID, "/appliance/uplinks/usage/byNetwork", q)
	if err != nil {
		return nil, err
	}
	p, err := fetchPage[ApplianceUplinkUsage](ctx, c, target, uplinkUsageOperation)
	if err != nil {
		return nil, err
	}
	return p.items, nil
}

func windowQuery(start, end time.Time, maximum time.Duration) (url.Values, error) {
	if start.IsZero() || end.IsZero() || !end.After(start) || end.Sub(start) > maximum {
		return nil, errors.New("invalid window")
	}
	return url.Values{"t0": {start.UTC().Format(time.RFC3339Nano)}, "t1": {end.UTC().Format(time.RFC3339Nano)}}, nil
}

func (c *Client) trafficURL(op, resource, id, suffix string, query url.Values) (*url.URL, error) {
	if id == "" || id == "." || id == ".." {
		return nil, fmt.Errorf("%s: invalid identifier", op)
	}
	target, err := url.Parse(c.baseURL.String() + "/" + resource + "/" + url.PathEscape(id) + suffix)
	if err != nil {
		return nil, fmt.Errorf("%s: invalid operation path", op)
	}
	target.RawQuery = query.Encode()
	return target, nil
}

// switchUsagePageURL restores omitted fixed parameters without allowing a link
// to change the requested window. Only documented page controls are variable.
func (c *Client) switchUsagePageURL(target *url.URL, path string, fixed url.Values, duration time.Duration) (*url.URL, error) {
	if !c.trustedTarget(target) || target.EscapedPath() != path {
		return nil, errors.New("unsafe pagination scope")
	}
	q, err := url.ParseQuery(target.RawQuery)
	if err != nil {
		return nil, err
	}
	for name, values := range q {
		if len(values) != 1 {
			return nil, errors.New("ambiguous pagination query")
		}
		switch name {
		case "startingAfter", "endingBefore":
		case "perPage":
			n, err := strconv.Atoi(values[0])
			if err != nil || n < minPageSize || n > 50 {
				return nil, errors.New("invalid page size")
			}
			q.Set(name, strconv.Itoa(n))
		case "t0", "t1":
			requested, requestedErr := time.Parse(time.RFC3339Nano, fixed.Get(name))
			linked, linkedErr := time.Parse(time.RFC3339Nano, values[0])
			if requestedErr != nil || linkedErr != nil || !linked.Equal(requested) {
				return nil, errors.New("changed time window")
			}
		case "interval":
			// A 300-second interval is unambiguous only for our 300s window.
			if duration != 300*time.Second || values[0] != "300" {
				return nil, errors.New("changed interval")
			}
			q.Del(name)
		default:
			return nil, errors.New("unexpected pagination query parameter")
		}
	}
	for name, values := range fixed {
		q.Set(name, values[0])
	}
	canonical := *target
	canonical.RawQuery = q.Encode()
	return &canonical, nil
}

func decodeItemsEnvelope[T any](r io.Reader, opName string) ([]T, error) {
	dec := json.NewDecoder(r)
	var envelope *struct {
		Items json.RawMessage `json:"items"`
	}
	if err := dec.Decode(&envelope); err != nil {
		return nil, safeError(opName, "invalid JSON items response", err)
	}
	if envelope == nil || len(envelope.Items) == 0 {
		return nil, fmt.Errorf("%s: expected a non-null items envelope", opName)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, safeError(opName, "unexpected trailing response data", err)
	}
	return decodeArray[T](bytes.NewReader(envelope.Items), opName)
}
