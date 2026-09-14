// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"encoding/xml"
	"fmt"
	"strings"
)

type panosResultResponse struct {
	XMLName xml.Name             `xml:"response"`
	Status  string               `xml:"status,attr"`
	Code    string               `xml:"code,attr"`
	Message panosResponseMessage `xml:"msg"`
	Result  struct {
		Message  panosResponseMessage `xml:"msg"`
		InnerXML string               `xml:",innerxml"`
	} `xml:"result"`
}

func decodePANOSResult(body []byte, context string, dst any) error {
	innerXML, err := decodePANOSResultInner(body, context)
	if err != nil {
		return err
	}
	if strings.TrimSpace(innerXML) == "" || dst == nil {
		return nil
	}

	wrapped := []byte("<result>" + innerXML + "</result>")
	if err := xml.Unmarshal(wrapped, dst); err != nil {
		return fmt.Errorf("parse %s result: %w", context, err)
	}
	return nil
}

func decodePANOSResultInner(body []byte, context string) (string, error) {
	var resp panosResultResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse %s: %w", context, err)
	}
	if resp.failed() {
		return "", panosResponseError{code: resp.Code, message: resp.errorMessage()}
	}
	return resp.Result.InnerXML, nil
}

func (r panosResultResponse) failed() bool {
	status := strings.ToLower(strings.TrimSpace(r.Status))
	if status == "error" || status == "failed" {
		return true
	}
	code := strings.TrimSpace(r.Code)
	// PAN-OS XML API uses 19 and 20 for successful command and operation responses.
	if code == "" || code == "0" || code == "19" || code == "20" {
		return false
	}
	return true
}

func (r panosResultResponse) errorMessage() string {
	return firstNonEmpty(r.Message.String(), r.Result.Message.String(), panosResponseCodeName(r.Code))
}

type missingPANOSResultError struct {
	expected string
}

func (e missingPANOSResultError) Error() string {
	return fmt.Sprintf("PAN-OS XML API success response has no recognized telemetry payload; expected %s", e.expected)
}
