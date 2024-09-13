// SPDX-License-Identifier: GPL-3.0-or-later

package squidlog

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// https://wiki.squid-cache.org/Features/LogFormat
// http://www.squid-cache.org/Doc/config/logformat/
// https://wiki.squid-cache.org/SquidFaq/SquidLogs#Squid_result_codes
// https://www.websense.com/content/support/library/web/v773/wcg_help/squid.aspx

/*
4.6.1:
logformat squid      %ts.%03tu %6tr %>a %Ss/%03>Hs %<st %rm %ru %[un %Sh/%<a %mt
logformat common     %>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st %Ss:%Sh
logformat combined   %>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st "%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh
logformat referrer   %ts.%03tu %>a %{Referer}>h %ru
logformat useragent  %>a [%tl] "%{User-Agent}>h"
logformat icap_squid %ts.%03tu %6icap::tr %>A %icap::to/%03icap::Hs %icap::<st %icap::rm %icap::ru %un -/%icap::<A -
*/

/*
Valid Capture Name: [A-Za-z0-9_]+
// TODO: namings

| local                   | squid format code | description                                                            |
|-------------------------|-------------------|------------------------------------------------------------------------|
| resp_time               | %tr               | Response time (milliseconds).
| client_address          | %>a               | Client source IP address.
| client_address          | %>A               | Client FQDN.
| cache_code              | %Ss               | Squid request status (TCP_MISS etc).
| http_code               | %>Hs              | The HTTP response status code from Content Gateway to client.
| resp_size               | %<st              | Total size of reply sent to client (after adaptation).
| req_method              | %rm               | Request method (GET/POST etc).
| hier_code               | %Sh               | Squid hierarchy status (DEFAULT_PARENT etc).
| server_address          | %<a               | Server IP address of the last server or peer connection.
| server_address          | %<A               | Server FQDN or peer name.
| mime_type               | %mt               | MIME content type.

// Following needed to make default log format csv parsable
| result_code             | %Ss/%03>Hs        | cache code and http code.
| hierarchy               | %Sh/%<a           | hierarchy code and server address.

Notes:
- %<a: older versions of Squid would put the origin server hostname here.
*/

var (
	errEmptyLine     = errors.New("empty line")
	errBadRespTime   = errors.New("bad response time")
	errBadClientAddr = errors.New("bad client address")
	errBadCacheCode  = errors.New("bad cache code")
	errBadHTTPCode   = errors.New("bad http code")
	errBadRespSize   = errors.New("bad response size")
	errBadReqMethod  = errors.New("bad request method")
	errBadHierCode   = errors.New("bad hier code")
	errBadServerAddr = errors.New("bad server address")
	errBadMimeType   = errors.New("bad mime type")
	errBadResultCode = errors.New("bad result code")
	errBadHierarchy  = errors.New("bad hierarchy")
)

func newEmptyLogLine() *logLine {
	var l logLine
	l.reset()
	return &l
}

type (
	logLine struct {
		clientAddr string
		serverAddr string

		respTime int
		respSize int
		httpCode int

		reqMethod string
		mimeType  string

		cacheCode string
		hierCode  string
	}
)

const (
	fieldRespTime   = "resp_time"
	fieldClientAddr = "client_address"
	fieldCacheCode  = "cache_code"
	fieldHTTPCode   = "http_code"
	fieldRespSize   = "resp_size"
	fieldReqMethod  = "req_method"
	fieldHierCode   = "hier_code"
	fieldServerAddr = "server_address"
	fieldMimeType   = "mime_type"
	fieldResultCode = "result_code"
	fieldHierarchy  = "hierarchy"
)

func (l *logLine) Assign(field string, value string) (err error) {
	if value == "" {
		return
	}

	switch field {
	case fieldRespTime:
		err = l.assignRespTime(value)
	case fieldClientAddr:
		err = l.assignClientAddress(value)
	case fieldCacheCode:
		err = l.assignCacheCode(value)
	case fieldHTTPCode:
		err = l.assignHTTPCode(value)
	case fieldRespSize:
		err = l.assignRespSize(value)
	case fieldReqMethod:
		err = l.assignReqMethod(value)
	case fieldHierCode:
		err = l.assignHierCode(value)
	case fieldMimeType:
		err = l.assignMimeType(value)
	case fieldServerAddr:
		err = l.assignServerAddress(value)
	case fieldResultCode:
		err = l.assignResultCode(value)
	case fieldHierarchy:
		err = l.assignHierarchy(value)
	}
	return err
}

const hyphen = "-"

func (l *logLine) assignRespTime(time string) error {
	if time == hyphen {
		return fmt.Errorf("assign '%s': %w", time, errBadRespTime)
	}
	v, err := strconv.Atoi(time)
	if err != nil || !isRespTimeValid(v) {
		return fmt.Errorf("assign '%s': %w", time, errBadRespTime)
	}
	l.respTime = v
	return nil
}

func (l *logLine) assignClientAddress(address string) error {
	if address == hyphen {
		return fmt.Errorf("assign '%s': %w", address, errBadClientAddr)
	}
	l.clientAddr = address
	return nil
}

func (l *logLine) assignCacheCode(code string) error {
	if code == hyphen || !isCacheCodeValid(code) {
		return fmt.Errorf("assign '%s': %w", code, errBadCacheCode)
	}
	l.cacheCode = code
	return nil
}

func (l *logLine) assignHTTPCode(code string) error {
	if code == hyphen {
		return fmt.Errorf("assign '%s': %w", code, errBadHTTPCode)
	}
	v, err := strconv.Atoi(code)
	if err != nil || !isHTTPCodeValid(v) {
		return fmt.Errorf("assign '%s': %w", code, errBadHTTPCode)
	}
	l.httpCode = v
	return nil
}

func (l *logLine) assignResultCode(code string) error {
	i := strings.IndexByte(code, '/')
	if i <= 0 {
		return fmt.Errorf("assign '%s': %w", code, errBadResultCode)
	}
	if err := l.assignCacheCode(code[:i]); err != nil {
		return err
	}
	return l.assignHTTPCode(code[i+1:])
}

func (l *logLine) assignRespSize(size string) error {
	if size == hyphen {
		return fmt.Errorf("assign '%s': %w", size, errBadRespSize)
	}
	v, err := strconv.Atoi(size)
	if err != nil || !isRespSizeValid(v) {
		return fmt.Errorf("assign '%s': %w", size, errBadRespSize)
	}
	l.respSize = v
	return nil
}

func (l *logLine) assignReqMethod(method string) error {
	if method == hyphen || !isReqMethodValid(method) {
		return fmt.Errorf("assign '%s': %w", method, errBadReqMethod)
	}
	l.reqMethod = method
	return nil
}

func (l *logLine) assignHierCode(code string) error {
	if code == hyphen || !isHierCodeValid(code) {
		return fmt.Errorf("assign '%s': %w", code, errBadHierCode)
	}
	l.hierCode = code
	return nil
}

func (l *logLine) assignServerAddress(address string) error {
	// Logged as "-" if there is no hierarchy information.
	// For TCP HIT, TCP failures, cachemgr requests and all UDP requests, there is no hierarchy information.
	if address == hyphen {
		return nil
	}
	l.serverAddr = address
	return nil
}

func (l *logLine) assignHierarchy(hierarchy string) error {
	i := strings.IndexByte(hierarchy, '/')
	if i <= 0 {
		return fmt.Errorf("assign '%s': %w", hierarchy, errBadHierarchy)
	}
	if err := l.assignHierCode(hierarchy[:i]); err != nil {
		return err
	}
	return l.assignServerAddress(hierarchy[i+1:])
}

func (l *logLine) assignMimeType(mime string) error {
	// ICP exchanges usually don't have any content type, and thus are logged "-".
	//Also, some weird replies have content types ":" or even empty ones.
	if mime == hyphen || mime == ":" {
		return nil
	}
	// format: type/subtype, type/subtype;parameter=value
	i := strings.IndexByte(mime, '/')
	if i <= 0 {
		return fmt.Errorf("assign '%s': %w", mime, errBadMimeType)
	}

	if !isMimeTypeValid(mime[:i]) {
		return nil
	}

	l.mimeType = mime[:i] // drop subtype

	return nil
}

func (l logLine) verify() error {
	if l.empty() {
		return fmt.Errorf("verify: %w", errEmptyLine)
	}
	if l.hasRespTime() && !l.isRespTimeValid() {
		return fmt.Errorf("verify '%d': %w", l.respTime, errBadRespTime)
	}
	if l.hasClientAddress() && !l.isClientAddressValid() {
		return fmt.Errorf("verify '%s': %w", l.clientAddr, errBadClientAddr)
	}
	if l.hasCacheCode() && !l.isCacheCodeValid() {
		return fmt.Errorf("verify '%s': %w", l.cacheCode, errBadCacheCode)
	}
	if l.hasHTTPCode() && !l.isHTTPCodeValid() {
		return fmt.Errorf("verify '%d': %w", l.httpCode, errBadHTTPCode)
	}
	if l.hasRespSize() && !l.isRespSizeValid() {
		return fmt.Errorf("verify '%d': %w", l.respSize, errBadRespSize)
	}
	if l.hasReqMethod() && !l.isReqMethodValid() {
		return fmt.Errorf("verify '%s': %w", l.reqMethod, errBadReqMethod)
	}
	if l.hasHierCode() && !l.isHierCodeValid() {
		return fmt.Errorf("verify '%s': %w", l.hierCode, errBadHierCode)
	}
	if l.hasServerAddress() && !l.isServerAddressValid() {
		return fmt.Errorf("verify '%s': %w", l.serverAddr, errBadServerAddr)
	}
	if l.hasMimeType() && !l.isMimeTypeValid() {
		return fmt.Errorf("verify '%s': %w", l.mimeType, errBadMimeType)
	}
	return nil
}

func (l logLine) empty() bool                { return l == emptyLogLine }
func (l logLine) hasRespTime() bool          { return !isEmptyNumber(l.respTime) }
func (l logLine) hasClientAddress() bool     { return !isEmptyString(l.clientAddr) }
func (l logLine) hasCacheCode() bool         { return !isEmptyString(l.cacheCode) }
func (l logLine) hasHTTPCode() bool          { return !isEmptyNumber(l.httpCode) }
func (l logLine) hasRespSize() bool          { return !isEmptyNumber(l.respSize) }
func (l logLine) hasReqMethod() bool         { return !isEmptyString(l.reqMethod) }
func (l logLine) hasHierCode() bool          { return !isEmptyString(l.hierCode) }
func (l logLine) hasServerAddress() bool     { return !isEmptyString(l.serverAddr) }
func (l logLine) hasMimeType() bool          { return !isEmptyString(l.mimeType) }
func (l logLine) isRespTimeValid() bool      { return isRespTimeValid(l.respTime) }
func (l logLine) isClientAddressValid() bool { return reAddress.MatchString(l.clientAddr) }
func (l logLine) isCacheCodeValid() bool     { return isCacheCodeValid(l.cacheCode) }
func (l logLine) isHTTPCodeValid() bool      { return isHTTPCodeValid(l.httpCode) }
func (l logLine) isRespSizeValid() bool      { return isRespSizeValid(l.respSize) }
func (l logLine) isReqMethodValid() bool     { return isReqMethodValid(l.reqMethod) }
func (l logLine) isHierCodeValid() bool      { return isHierCodeValid(l.hierCode) }
func (l logLine) isServerAddressValid() bool { return reAddress.MatchString(l.serverAddr) }
func (l logLine) isMimeTypeValid() bool      { return isMimeTypeValid(l.mimeType) }

func (l *logLine) reset() {
	l.respTime = emptyNumber
	l.clientAddr = emptyString
	l.cacheCode = emptyString
	l.httpCode = emptyNumber
	l.respSize = emptyNumber
	l.reqMethod = emptyString
	l.hierCode = emptyString
	l.serverAddr = emptyString
	l.mimeType = emptyString
}

var emptyLogLine = *newEmptyLogLine()

const (
	emptyString = "__empty_string__"
	emptyNumber = -9999
)

var (
	// IPv4, IPv6, FQDN.
	reAddress = regexp.MustCompile(`^(?:(?:[0-9]{1,3}\.){3}[0-9]{1,3}|[a-f0-9:]{3,}|[a-zA-Z0-9-.]{3,})$`)
)

func isEmptyString(s string) bool {
	return s == emptyString || s == ""
}

func isEmptyNumber(n int) bool {
	return n == emptyNumber
}

func isRespTimeValid(time int) bool {
	return time >= 0
}

// isCacheCodeValid does not guarantee cache result code is valid, but it is very likely.
func isCacheCodeValid(code string) bool {
	// https://wiki.squid-cache.org/SquidFaq/SquidLogs#Squid_result_codes
	if code == "NONE" || code == "NONE_NONE" {
		return true
	}
	return len(code) > 5 && (code[:4] == "TCP_" || code[:4] == "UDP_")
}

func isHTTPCodeValid(code int) bool {
	// https://wiki.squid-cache.org/SquidFaq/SquidLogs#HTTP_status_codes
	return code == 0 || code >= 100 && code <= 603
}

func isRespSizeValid(size int) bool {
	return size >= 0
}

func isReqMethodValid(method string) bool {
	// https://wiki.squid-cache.org/SquidFaq/SquidLogs#Request_methods
	switch method {
	case "GET",
		"HEAD",
		"POST",
		"PUT",
		"PATCH",
		"DELETE",
		"CONNECT",
		"OPTIONS",
		"TRACE",
		"ICP_QUERY",
		"PURGE",
		"PROPFIND",
		"PROPATCH",
		"MKCOL",
		"COPY",
		"MOVE",
		"LOCK",
		"UNLOCK",
		"NONE":
		return true
	}
	return false
}

// isHierCodeValid does not guarantee hierarchy code is valid, but it is very likely.
func isHierCodeValid(code string) bool {
	// https://wiki.squid-cache.org/SquidFaq/SquidLogs#Hierarchy_Codes
	return len(code) > 6 && code[:5] == "HIER_"
}

// isMimeTypeValid expects only mime type part.
func isMimeTypeValid(mimeType string) bool {
	// https://www.iana.org/assignments/media-types/media-types.xhtml
	if mimeType == "text" {
		return true
	}
	switch mimeType {
	case "application", "audio", "font", "image", "message", "model", "multipart", "video":
		return true
	}
	return false
}
