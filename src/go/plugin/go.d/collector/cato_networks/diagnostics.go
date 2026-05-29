// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"context"
	"errors"
	"net"
	"strings"
)

const (
	operationDiscovery = "entityLookup"
	operationSnapshot  = "accountSnapshot"
	operationMetrics   = "accountMetrics"
	operationBGP       = "siteBgpStatus"
)

const (
	normalizationSurfaceSiteConnectivity = "site_connectivity"
	normalizationSurfaceSiteOperational  = "site_operational"
	normalizationSurfaceMetrics          = "metrics"
	normalizationSurfaceBGP              = "bgp"
)

const (
	normalizationIssueUnknownStatus          = "unknown_status"
	normalizationIssueUnknownTimeseriesLabel = "unknown_timeseries_label"
	normalizationIssueEmptyPeer              = "empty_peer"
	normalizationIssueParseInt               = "parse_int"
	normalizationIssueAccountError           = "account_error"
	normalizationIssuePageCap                = "page_cap"
)

type errorClasser interface {
	ErrorClass() string
}

type classifiedError struct {
	message string
	class   string
	cause   error
}

func wrapCatoOperationError(message string, err error) error {
	if err == nil {
		return nil
	}
	return classifiedError{
		message: message + " failed",
		class:   classifyCatoError(err),
		cause:   err,
	}
}

func (e classifiedError) Error() string {
	return e.message + ", error_class=" + e.class
}

func (e classifiedError) Unwrap() error {
	return e.cause
}

func (e classifiedError) ErrorClass() string {
	return e.class
}

func classifyCatoError(err error) string {
	if err == nil {
		return "none"
	}
	var classified errorClasser
	if errors.As(err, &classified) {
		return classified.ErrorClass()
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}

	msg := strings.ToLower(err.Error())
	switch {
	case containsAny(msg, "rate limit", "rate-limit", "ratelimit", "too many requests"),
		containsHTTPStatus(msg, "429"):
		return "rate_limit"
	case containsAny(msg, "unauthorized", "forbidden", "invalid api key"),
		containsHTTPStatus(msg, "401"),
		containsHTTPStatus(msg, "403"):
		return "auth"
	case containsAny(msg, "json", "decode", "unmarshal"):
		return "decode"
	case containsAny(msg, "deadline", "timeout", "i/o timeout"):
		return "timeout"
	case containsAny(msg, "proxyconnect", "proxy error", "proxy url", "proxy"):
		return "proxy"
	case containsAny(msg, "x509", "certificate", "tls", "handshake"):
		return "tls"
	case containsAny(msg, "no such host", "connection refused", "connection reset", "network is unreachable",
		"temporary failure in name resolution", "server misbehaving", "dial tcp", "eof"):
		return "network"
	case containsAny(msg, "no cato sites", "no sites"):
		return "empty"
	case strings.Contains(msg, "pagination"):
		return "pagination"
	case strings.Contains(msg, "graphql"):
		return "graphql"
	default:
		return "error"
	}
}

func containsAny(s string, substrings ...string) bool {
	for _, substring := range substrings {
		if strings.Contains(s, substring) {
			return true
		}
	}
	return false
}

func containsHTTPStatus(msg, code string) bool {
	return strings.Contains(msg, "http status "+code) ||
		strings.Contains(msg, "http "+code) ||
		strings.Contains(msg, "status code "+code) ||
		strings.Contains(msg, "status "+code) ||
		strings.Contains(msg, " "+code+" ")
}
