// SPDX-License-Identifier: GPL-3.0-or-later

package weblog

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// TODO: it is not clear how to handle "-", current handling is not good
// In general it is:
//   - If a field is unused in a particular entry dash "-" marks the omitted field.
// In addition to that "-" is used as zero value in:
//   - apache: %b '-' when no bytes are sent.
//
// Log Format:
//  - CLF: https://www.w3.org/Daemon/User/Config/Logging.html#common-logfile-format
//  - ELF: https://www.w3.org/TR/WD-logfile.html
//  - Apache CLF: https://httpd.apache.org/docs/trunk/logs.html#common

// Variables:
//  - nginx: http://nginx.org/en/docs/varindex.html
//  - apache: http://httpd.apache.org/docs/current/mod/mod_log_config.html#logformat
//  - IIS: https://learn.microsoft.com/en-us/windows/win32/http/w3c-logging

/*
| nginx                   | apache    | description                                   |
|-------------------------|-----------|-----------------------------------------------|
| $host ($http_host)      | %v        | Name of the server which accepted a request.
| $server_port            | %p        | Port of the server which accepted a request.
| $scheme                 | -         | Request scheme. "http" or "https".
| $remote_addr            | %a (%h)   | Client address.
| $request                | %r        | Full original request line. The line is "$request_method $request_uri $server_protocol".
| $request_method         | %m        | Request method. Usually "GET" or "POST".
| $request_uri            | %U        | Full original request URI.
| $server_protocol        | %H        | Request protocol. Usually "HTTP/1.0", "HTTP/1.1", or "HTTP/2.0".
| $status                 | %s (%>s)  | Response status code.
| $request_length         | %I        | Bytes received from a client, including request and headers.
| $bytes_sent             | %O        | Bytes sent to a client, including request and headers.
| $body_bytes_sent        | %B (%b)   | Bytes sent to a client, not counting the response header.
| $request_time           | %D        | Request processing time.
| $upstream_response_time | -         | Time spent on receiving the response from the upstream server.
| $ssl_protocol           | -         | Protocol of an established SSL connection.
| $ssl_cipher             | -         | String of ciphers used for an established SSL connection.
*/

var (
	errEmptyLine         = errors.New("empty line")
	errBadVhost          = errors.New("bad vhost")
	errBadVhostPort      = errors.New("bad vhost with port")
	errBadPort           = errors.New("bad port")
	errBadReqScheme      = errors.New("bad req scheme")
	errBadReqClient      = errors.New("bad req client")
	errBadRequest        = errors.New("bad request")
	errBadReqMethod      = errors.New("bad req method")
	errBadReqURL         = errors.New("bad req url")
	errBadReqProto       = errors.New("bad req protocol")
	errBadReqSize        = errors.New("bad req size")
	errBadRespCode       = errors.New("bad resp status code")
	errBadRespSize       = errors.New("bad resp size")
	errBadReqProcTime    = errors.New("bad req processing time")
	errBadUpsRespTime    = errors.New("bad upstream resp time")
	errBadSSLProto       = errors.New("bad ssl protocol")
	errBadSSLCipherSuite = errors.New("bad ssl cipher suite")
)

func newEmptyLogLine() *logLine {
	var l logLine
	l.custom.fields = make(map[string]struct{})
	l.custom.values = make([]customValue, 0, 20)
	l.reset()
	return &l
}

type (
	logLine struct {
		web
		custom custom
	}
	web struct {
		vhost          string
		port           string
		reqScheme      string
		reqClient      string
		reqMethod      string
		reqURL         string
		reqProto       string
		reqSize        int
		reqProcTime    float64
		respCode       int
		respSize       int
		upsRespTime    float64
		sslProto       string
		sslCipherSuite string
	}
	custom struct {
		fields map[string]struct{}
		values []customValue
	}
	customValue struct {
		name  string
		value string
	}
)

func (l *logLine) Assign(field string, value string) (err error) {
	if value == "" {
		return
	}

	switch field {
	case "host", "http_host", "v":
		err = l.assignVhost(value)
	case "server_port", "p":
		err = l.assignPort(value)
	case "host:$server_port", "v:%p":
		err = l.assignVhostWithPort(value)
	case "scheme":
		err = l.assignReqScheme(value)
	case "remote_addr", "a", "h":
		err = l.assignReqClient(value)
	case "request", "r":
		err = l.assignRequest(value)
	case "request_method", "m":
		err = l.assignReqMethod(value)
	case "request_uri", "U":
		err = l.assignReqURL(value)
	case "server_protocol", "H":
		err = l.assignReqProto(value)
	case "status", "s", ">s":
		err = l.assignRespCode(value)
	case "request_length", "I":
		err = l.assignReqSize(value)
	case "bytes_sent", "body_bytes_sent", "b", "O", "B":
		err = l.assignRespSize(value)
	case "request_time", "D":
		err = l.assignReqProcTime(value)
	case "upstream_response_time":
		err = l.assignUpsRespTime(value)
	case "ssl_protocol":
		err = l.assignSSLProto(value)
	case "ssl_cipher":
		err = l.assignSSLCipherSuite(value)
	default:
		err = l.assignCustom(field, value)
	}
	if err != nil {
		err = fmt.Errorf("assign '%s': %w", field, err)
	}
	return err
}

const hyphen = "-"

func (l *logLine) assignVhost(vhost string) error {
	if vhost == hyphen {
		return nil
	}
	// nginx $host and $http_host returns ipv6 in [], apache not
	if idx := strings.IndexByte(vhost, ']'); idx > 0 {
		vhost = vhost[1:idx]
	}
	l.vhost = vhost
	return nil
}

func (l *logLine) assignPort(port string) error {
	if port == hyphen {
		return nil
	}
	if !isPortValid(port) {
		return fmt.Errorf("assign '%s' : %w", port, errBadPort)
	}
	l.port = port
	return nil
}

func (l *logLine) assignVhostWithPort(vhostPort string) error {
	if vhostPort == hyphen {
		return nil
	}
	idx := strings.LastIndexByte(vhostPort, ':')
	if idx == -1 {
		return fmt.Errorf("assign '%s' : %w", vhostPort, errBadVhostPort)
	}
	if err := l.assignPort(vhostPort[idx+1:]); err != nil {
		return fmt.Errorf("assign '%s' : %w", vhostPort, errBadVhostPort)
	}
	if err := l.assignVhost(vhostPort[0:idx]); err != nil {
		return fmt.Errorf("assign '%s' : %w", vhostPort, errBadVhostPort)
	}
	return nil
}

func (l *logLine) assignReqScheme(scheme string) error {
	if scheme == hyphen {
		return nil
	}
	if !isSchemeValid(scheme) {
		return fmt.Errorf("assign '%s' : %w", scheme, errBadReqScheme)
	}
	l.reqScheme = scheme
	return nil
}

func (l *logLine) assignReqClient(client string) error {
	if client == hyphen {
		return nil
	}
	l.reqClient = client
	return nil
}

func (l *logLine) assignRequest(request string) error {
	if request == hyphen {
		return nil
	}
	var first, last int
	if first = strings.IndexByte(request, ' '); first < 0 {
		return fmt.Errorf("assign '%s': %w", request, errBadRequest)
	}
	if last = strings.LastIndexByte(request, ' '); first == last {
		return fmt.Errorf("assign '%s': %w", request, errBadRequest)
	}
	proto := request[last+1:]
	url := request[first+1 : last]
	method := request[0:first]
	if err := l.assignReqMethod(method); err != nil {
		return err
	}
	if err := l.assignReqURL(url); err != nil {
		return err
	}
	return l.assignReqProto(proto)
}

func (l *logLine) assignReqMethod(method string) error {
	if method == hyphen {
		return nil
	}
	if !isReqMethodValid(method) {
		return fmt.Errorf("assign '%s' : %w", method, errBadReqMethod)
	}
	l.reqMethod = method
	return nil
}

func (l *logLine) assignReqURL(url string) error {
	if url == hyphen {
		return nil
	}
	if isEmptyString(url) {
		return fmt.Errorf("assign '%s' : %w", url, errBadReqURL)
	}
	l.reqURL = url
	return nil
}

func (l *logLine) assignReqProto(proto string) error {
	if proto == hyphen {
		return nil
	}
	if !isReqProtoValid(proto) {
		return fmt.Errorf("assign '%s': %w", proto, errBadReqProto)
	}
	l.reqProto = proto[5:]
	return nil
}

func (l *logLine) assignRespCode(status string) error {
	if status == hyphen {
		return nil
	}
	v, err := strconv.Atoi(status)
	if err != nil || !isRespCodeValid(v) {
		return fmt.Errorf("assign '%s': %w", status, errBadRespCode)
	}
	l.respCode = v
	return nil
}

func (l *logLine) assignReqSize(size string) error {
	// apache: can be "-" according web_log py regexp.
	if size == hyphen {
		l.reqSize = 0
		return nil
	}
	v, err := strconv.Atoi(size)
	if err != nil || !isSizeValid(v) {
		return fmt.Errorf("assign '%s': %w", size, errBadReqSize)
	}
	l.reqSize = v
	return nil
}

func (l *logLine) assignRespSize(size string) error {
	// apache: %b. In CLF format, i.e. a '-' rather than a 0 when no bytes are sent.
	if size == hyphen {
		l.respSize = 0
		return nil
	}
	v, err := strconv.Atoi(size)
	if err != nil || !isSizeValid(v) {
		return fmt.Errorf("assign '%s': %w", size, errBadRespSize)
	}
	l.respSize = v
	return nil
}

func (l *logLine) assignReqProcTime(time string) error {
	if time == hyphen {
		return nil
	}
	if time == "0.000" {
		l.reqProcTime = 0
		return nil
	}
	v, err := strconv.ParseFloat(time, 64)
	if err != nil || !isTimeValid(v) {
		return fmt.Errorf("assign '%s': %w", time, errBadReqProcTime)
	}
	l.reqProcTime = v * timeMultiplier(time)
	return nil
}

func isUpstreamTimeSeparator(r rune) bool { return r == ',' || r == ':' }

func (l *logLine) assignUpsRespTime(time string) error {
	if time == hyphen {
		return nil
	}

	// the upstream response time string can contain multiple values, separated
	// by commas (in case the request was handled by multiple servers), or colons
	// (in case the request passed between multiple server groups via an internal redirect)
	// the individual values should be summed up to obtain the correct amount of time
	// the request spent in upstream
	var sum float64
	for _, val := range strings.FieldsFunc(time, isUpstreamTimeSeparator) {
		val = strings.TrimSpace(val)
		v, err := strconv.ParseFloat(val, 64)
		if err != nil || !isTimeValid(v) {
			return fmt.Errorf("assign '%s': %w", time, errBadUpsRespTime)
		}

		sum += v
	}

	l.upsRespTime = sum * timeMultiplier(time)
	return nil
}

func (l *logLine) assignSSLProto(proto string) error {
	if proto == hyphen {
		return nil
	}
	if !isSSLProtoValid(proto) {
		return fmt.Errorf("assign '%s': %w", proto, errBadSSLProto)
	}
	l.sslProto = proto
	return nil
}

func (l *logLine) assignSSLCipherSuite(cipher string) error {
	if cipher == hyphen {
		return nil
	}
	if strings.IndexByte(cipher, '-') <= 0 && strings.IndexByte(cipher, '_') <= 0 {
		return fmt.Errorf("assign '%s': %w", cipher, errBadSSLCipherSuite)
	}
	l.sslCipherSuite = cipher
	return nil
}

func (l *logLine) assignCustom(field, value string) error {
	if len(l.custom.fields) == 0 || value == hyphen {
		return nil
	}
	if _, ok := l.custom.fields[field]; ok {
		l.custom.values = append(l.custom.values, customValue{name: field, value: value})
	}
	return nil
}

func (l *logLine) verify() error {
	if l.empty() {
		return fmt.Errorf("verify: %w", errEmptyLine)
	}
	if l.hasRespCode() && !l.isRespCodeValid() {
		return fmt.Errorf("verify '%d': %w", l.respCode, errBadRespCode)
	}
	if l.hasVhost() && !l.isVhostValid() {
		return fmt.Errorf("verify '%s': %w", l.vhost, errBadVhost)
	}
	if l.hasPort() && !l.isPortValid() {
		return fmt.Errorf("verify '%s': %w", l.port, errBadPort)
	}
	if l.hasReqScheme() && !l.isSchemeValid() {
		return fmt.Errorf("verify '%s': %w", l.reqScheme, errBadReqScheme)
	}
	if l.hasReqClient() && !l.isClientValid() {
		return fmt.Errorf("verify '%s': %w", l.reqClient, errBadReqClient)
	}
	if l.hasReqMethod() && !l.isMethodValid() {
		return fmt.Errorf("verify '%s': %w", l.reqMethod, errBadReqMethod)
	}
	if l.hasReqURL() && !l.isURLValid() {
		return fmt.Errorf("verify '%s': %w", l.reqURL, errBadReqURL)
	}
	if l.hasReqProto() && !l.isProtoValid() {
		return fmt.Errorf("verify '%s': %w", l.reqProto, errBadReqProto)
	}
	if l.hasReqSize() && !l.isReqSizeValid() {
		return fmt.Errorf("verify '%d': %w", l.reqSize, errBadReqSize)
	}
	if l.hasRespSize() && !l.isRespSizeValid() {
		return fmt.Errorf("verify '%d': %w", l.respSize, errBadRespSize)
	}
	if l.hasReqProcTime() && !l.isReqProcTimeValid() {
		return fmt.Errorf("verify '%f': %w", l.reqProcTime, errBadReqProcTime)
	}
	if l.hasUpsRespTime() && !l.isUpsRespTimeValid() {
		return fmt.Errorf("verify '%f': %w", l.upsRespTime, errBadUpsRespTime)
	}
	if l.hasSSLProto() && !l.isSSLProtoValid() {
		return fmt.Errorf("verify '%s': %w", l.sslProto, errBadSSLProto)
	}
	if l.hasSSLCipherSuite() && !l.isSSLCipherSuiteValid() {
		return fmt.Errorf("verify '%s': %w", l.sslCipherSuite, errBadSSLCipherSuite)
	}
	return nil
}

func (l *logLine) empty() bool                 { return !l.hasWebFields() && !l.hasCustomFields() }
func (l *logLine) hasCustomFields() bool       { return len(l.custom.values) > 0 }
func (l *logLine) hasWebFields() bool          { return l.web != emptyWebFields }
func (l *logLine) hasVhost() bool              { return !isEmptyString(l.vhost) }
func (l *logLine) hasPort() bool               { return !isEmptyString(l.port) }
func (l *logLine) hasReqScheme() bool          { return !isEmptyString(l.reqScheme) }
func (l *logLine) hasReqClient() bool          { return !isEmptyString(l.reqClient) }
func (l *logLine) hasReqMethod() bool          { return !isEmptyString(l.reqMethod) }
func (l *logLine) hasReqURL() bool             { return !isEmptyString(l.reqURL) }
func (l *logLine) hasReqProto() bool           { return !isEmptyString(l.reqProto) }
func (l *logLine) hasRespCode() bool           { return !isEmptyNumber(l.respCode) }
func (l *logLine) hasReqSize() bool            { return !isEmptyNumber(l.reqSize) }
func (l *logLine) hasRespSize() bool           { return !isEmptyNumber(l.respSize) }
func (l *logLine) hasReqProcTime() bool        { return !isEmptyNumber(int(l.reqProcTime)) }
func (l *logLine) hasUpsRespTime() bool        { return !isEmptyNumber(int(l.upsRespTime)) }
func (l *logLine) hasSSLProto() bool           { return !isEmptyString(l.sslProto) }
func (l *logLine) hasSSLCipherSuite() bool     { return !isEmptyString(l.sslCipherSuite) }
func (l *logLine) isVhostValid() bool          { return reVhost.MatchString(l.vhost) }
func (l *logLine) isPortValid() bool           { return isPortValid(l.port) }
func (l *logLine) isSchemeValid() bool         { return isSchemeValid(l.reqScheme) }
func (l *logLine) isClientValid() bool         { return reClient.MatchString(l.reqClient) }
func (l *logLine) isMethodValid() bool         { return isReqMethodValid(l.reqMethod) }
func (l *logLine) isURLValid() bool            { return !isEmptyString(l.reqURL) }
func (l *logLine) isProtoValid() bool          { return isReqProtoVerValid(l.reqProto) }
func (l *logLine) isRespCodeValid() bool       { return isRespCodeValid(l.respCode) }
func (l *logLine) isReqSizeValid() bool        { return isSizeValid(l.reqSize) }
func (l *logLine) isRespSizeValid() bool       { return isSizeValid(l.respSize) }
func (l *logLine) isReqProcTimeValid() bool    { return isTimeValid(l.reqProcTime) }
func (l *logLine) isUpsRespTimeValid() bool    { return isTimeValid(l.upsRespTime) }
func (l *logLine) isSSLProtoValid() bool       { return isSSLProtoValid(l.sslProto) }
func (l *logLine) isSSLCipherSuiteValid() bool { return reCipherSuite.MatchString(l.sslCipherSuite) }

func (l *logLine) reset() {
	l.web = emptyWebFields
	l.custom.values = l.custom.values[:0]
}

var (
	// TODO: reClient doesn't work with %h when HostnameLookups is On.
	reVhost       = regexp.MustCompile(`^[a-zA-Z0-9-:.]+$`)
	reClient      = regexp.MustCompile(`^([\da-f:.]+|localhost)$`)
	reCipherSuite = regexp.MustCompile(`^[A-Z0-9-_]+$`) // openssl -v
)

var emptyWebFields = web{
	vhost:          emptyString,
	port:           emptyString,
	reqScheme:      emptyString,
	reqClient:      emptyString,
	reqMethod:      emptyString,
	reqURL:         emptyString,
	reqProto:       emptyString,
	reqSize:        emptyNumber,
	reqProcTime:    emptyNumber,
	respCode:       emptyNumber,
	respSize:       emptyNumber,
	upsRespTime:    emptyNumber,
	sslProto:       emptyString,
	sslCipherSuite: emptyString,
}

const (
	emptyString = "__empty_string__"
	emptyNumber = -9999
)

func isEmptyString(s string) bool {
	return s == emptyString || s == ""
}

func isEmptyNumber(n int) bool {
	return n == emptyNumber
}

func isReqMethodValid(method string) bool {
	// https://www.iana.org/assignments/http-methods/http-methods.xhtml
	switch method {
	case "GET",
		"ACL",
		"BASELINE-CONTROL",
		"BIND",
		"CHECKIN",
		"CHECKOUT",
		"CONNECT",
		"COPY",
		"DELETE",
		"HEAD",
		"LABEL",
		"LINK",
		"LOCK",
		"MERGE",
		"MKACTIVITY",
		"MKCALENDAR",
		"MKCOL",
		"MKREDIRECTREF",
		"MKWORKSPACE",
		"MOVE",
		"OPTIONS",
		"ORDERPATCH",
		"PATCH",
		"POST",
		"PRI",
		"PROPFIND",
		"PROPPATCH",
		"PURGE", // not a standardized HTTP method
		"PUT",
		"REBIND",
		"REPORT",
		"SEARCH",
		"TRACE",
		"UNBIND",
		"UNCHECKOUT",
		"UNLINK",
		"UNLOCK",
		"UPDATE",
		"UPDATEREDIRECTREF":
		return true
	}
	return false
}

func isReqProtoValid(proto string) bool {
	return len(proto) >= 6 && proto[:5] == "HTTP/" && isReqProtoVerValid(proto[5:])
}

func isReqProtoVerValid(version string) bool {
	switch version {
	case "1.1", "1", "1.0", "2", "2.0", "3", "3.0":
		return true
	}
	return false
}

func isPortValid(port string) bool {
	v, err := strconv.Atoi(port)
	return err == nil && v >= 80 && v <= 49151
}

func isSchemeValid(scheme string) bool {
	return scheme == "http" || scheme == "https"
}

func isRespCodeValid(code int) bool {
	// rfc7231
	// Informational responses (100–199),
	// Successful responses (200–299),
	// Redirects (300–399),
	// Client errors (400–499),
	// Server errors (500–599).
	return code >= 100 && code <= 600
}

func isSizeValid(size int) bool {
	return size >= 0
}

func isTimeValid(time float64) bool {
	return time >= 0
}

func isSSLProtoValid(proto string) bool {
	if proto == "TLSv1.2" {
		return true
	}
	switch proto {
	case "TLSv1.3", "SSLv2", "SSLv3", "TLSv1", "TLSv1.1":
		return true
	}
	return false
}

func timeMultiplier(time string) float64 {
	// TODO: Change code to detect and modify properly IIS time (in milliseconds)
	// Convert to microseconds:
	//   - nginx time is in seconds with a milliseconds' resolution.
	if strings.IndexByte(time, '.') > 0 {
		return 1e6
	}
	//   - apache time is in microseconds.
	return 1
}
