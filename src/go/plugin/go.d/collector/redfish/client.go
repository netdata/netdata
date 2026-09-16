// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"mime"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

const (
	maxResponseBodyBytes = 16 << 20
	maxErrorBodyBytes    = 64 << 10
	maxRedirects         = 3
	maxURIBytes          = 8192
	maxSessionTokenBytes = 8192
	maxContentTypeBytes  = 1024
	maxODataVersionBytes = 64
	maxRetryAfter        = 5 * time.Second
	maxRetryAfterBytes   = 128
	legacySessionsURI    = "/redfish/v1/Sessions"
)

type protocolClient struct {
	config      Config
	http        *http.Client
	root        *url.URL
	origin      string
	endpointJob string
	hardwareState
	semMu          sync.RWMutex
	sem            chan struct{}
	families       map[string]bool
	serviceMetaMu  sync.RWMutex
	serviceName    string
	redfishVersion string

	authMu      sync.RWMutex
	authMode    string
	token       string
	sessionURI  string
	sessionInit bool
	sessions    []sessionHandle
	refreshMu   sync.Mutex

	baseMu                sync.Mutex
	baseMembership        map[string][]baseResource
	graphMu               sync.Mutex
	graphMembership       map[string]graphMembershipSnapshot
	collectionMu          sync.Mutex
	knownCollections      map[string]struct{}
	expansionValue        string
	expansionDisabled     map[string]struct{}
	expansionFallbackSeen bool

	identities identityRegistry

	diagnosticMu             sync.Mutex
	pendingCompatibilityDiag map[string]struct{}
}

type wireStats struct {
	started    int
	retried    int
	redirected int
	received   int64
	successful int
	failed     int
	failures   map[string]int
	responses  map[string]struct{}
}

func (s *wireStats) merge(other *wireStats) {
	if s == nil || other == nil {
		return
	}
	s.started += other.started
	s.retried += other.retried
	s.redirected += other.redirected
	s.received += other.received
	s.successful += other.successful
	s.failed += other.failed
	if s.failures == nil {
		s.failures = make(map[string]int)
	}
	for class, count := range other.failures {
		s.failures[class] += count
	}
	if s.responses == nil {
		s.responses = make(map[string]struct{})
	}
	for key := range other.responses {
		s.responses[key] = struct{}{}
	}
}

type responseData struct {
	status     int
	header     http.Header
	body       []byte
	url        *url.URL
	startedAt  time.Time
	finishedAt time.Time
	stats      *wireStats
	finished   bool
}

func (r *responseData) finish(err error) {
	if r == nil || r.finished || r.stats == nil {
		return
	}
	r.finished = true
	if err == nil {
		r.stats.successful++
		recordResponseCompatibility(r.stats, r.header)
		return
	}
	r.stats.failed++
	if r.stats.failures == nil {
		r.stats.failures = make(map[string]int)
	}
	r.stats.failures[classifyError(err)]++
}

type responseMetadata struct {
	ContentTypeState  string
	ODataVersionState string
	StartedAt         time.Time
	FinishedAt        time.Time
}

type sessionHandle struct {
	token string
	uri   string
}

type statusError struct {
	status int
	class  string
	path   string
}

func (e statusError) Error() string {
	return fmt.Sprintf("Redfish request to %s returned HTTP %d", e.path, e.status)
}

type transportError struct {
	timeout   bool
	temporary bool
}

func (e transportError) Error() string {
	if e.timeout {
		return "Redfish transport timed out"
	}
	return "Redfish transport error"
}

func (e transportError) Timeout() bool   { return e.timeout }
func (e transportError) Temporary() bool { return e.temporary }

func newEndpointClient(cfg Config, client *http.Client) (endpointClient, error) {
	root, origin, err := normalizeServiceRoot(cfg.URL)
	if err != nil {
		return nil, err
	}
	familyMatcher, err := matcher.NewSimplePatternsMatcher(cfg.Collect)
	if err != nil {
		return nil, err
	}
	families := make(map[string]bool, len(collectionFamilies))
	for _, family := range collectionFamilies {
		families[family] = family == "base" || familyMatcher.MatchString(family)
	}
	result := &protocolClient{
		config:            cfg,
		endpointJob:       cfg.Name,
		http:              client,
		root:              root,
		origin:            origin,
		authMode:          cfg.AuthMethod,
		baseMembership:    make(map[string][]baseResource),
		graphMembership:   make(map[string]graphMembershipSnapshot),
		knownCollections:  make(map[string]struct{}),
		expansionDisabled: make(map[string]struct{}),
		sem:               make(chan struct{}, cfg.MaxConcurrentRequests),
		families:          families,
	}
	result.hardwareState.initialize()
	return result, nil
}

// Check identifies the endpoint without acquiring remote session state. Session
// credentials are validated when the running collector first authenticates.
func (c *protocolClient) Check(ctx context.Context) error {
	stats := &wireStats{failures: make(map[string]int)}
	defer c.rememberCompatibilityDiagnostics(stats)
	_, err := c.fetchServiceRoot(ctx, c.config.AuthMethod == "basic", stats)
	return err
}

func (c *protocolClient) initializeAuthentication(ctx context.Context, stats *wireStats) error {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()
	if c.authenticationInitialized() {
		return nil
	}
	switch c.config.AuthMethod {
	case "auto", "session":
		root, err := c.fetchServiceRoot(ctx, false, stats)
		if err != nil {
			return fmt.Errorf("read unauthenticated ServiceRoot for session discovery: %w", err)
		}
		if err := c.initializeSession(ctx, root, stats); err != nil {
			if c.config.AuthMethod != "auto" || !errors.Is(err, errSessionUnsupported) {
				return err
			}
			c.setAuth("basic", "", "")
		}
	case "basic", "none":
		c.setAuth(c.config.AuthMethod, "", "")
	default:
		return fmt.Errorf("unsupported authentication mode %q", c.config.AuthMethod)
	}
	return nil
}

func (c *protocolClient) authenticationInitialized() bool {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	return c.sessionInit
}

func (c *protocolClient) selectedAuthenticationMethod() string {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	if !c.sessionInit {
		return ""
	}
	return c.authMode
}

func (c *protocolClient) Collect(ctx context.Context) (collectionResult, error) {
	started := time.Now()
	stats := &wireStats{failures: make(map[string]int)}
	result := collectionResult{
		ObservedAt: time.Now().UTC(),
		Metrics: cycleMetrics{
			Failures:     make(map[string]int),
			HTTPRequests: make(map[string]int),
			Operations:   make(map[string]int),
			Resources:    make(map[string]int),
		},
	}
	result.Diagnostics = append(result.Diagnostics, c.takeCompatibilityDiagnostics()...)

	err := c.initializeAuthentication(ctx, stats)
	var root *serviceRootDocument
	if err == nil {
		root, err = c.fetchServiceRoot(ctx, true, stats)
	}
	if err != nil {
		result.Metrics.Status = "unavailable"
		result.Metrics.Duration = time.Since(started).Seconds()
		c.copyWireStats(&result.Metrics, stats)
		return result, err
	}
	resources, _, baseComplete, baseErr := c.fetchBaseResources(ctx, root, stats)
	graph, graphErr := c.collectResourceGraph(ctx, root, resources, stats)
	collectionErr := errors.Join(baseErr, graphErr)
	result.Complete = baseComplete && baseErr == nil && graph.Complete && graphErr == nil
	if identityIntegrityError(graphErr) {
		result.Complete = false
		result.Diagnostics = append(result.Diagnostics, boundedDiagnostic(graphErr.Error()))
		c.finishCollectionResult(&result, graph, stats, started)
		return result, collectionErr
	}
	var hardwareErr error
	result.Hardware, hardwareErr = c.hardwareSurface(graph, result.ObservedAt)
	collectionErr = errors.Join(collectionErr, hardwareErr)
	result.Complete = result.Complete && hardwareErr == nil
	result.Diagnostics = append(result.Diagnostics, graph.finalDiagnostics()...)
	if diagnostic := c.takeExpansionFallbackDiagnostic(); diagnostic != "" {
		result.Diagnostics = append(result.Diagnostics, diagnostic)
	}
	if hardwareErr != nil {
		result.Diagnostics = append(result.Diagnostics, boundedDiagnostic(
			"Redfish metric surface: "+hardwareErr.Error(),
		))
	}
	if identityIntegrityError(hardwareErr) {
		result.Hardware = nil
		result.Complete = false
		c.finishCollectionResult(&result, graph, stats, started)
		return result, collectionErr
	}
	c.finishCollectionResult(&result, graph, stats, started)
	return result, collectionErr
}

func (c *protocolClient) finishCollectionResult(
	result *collectionResult,
	graph *resourceGraph,
	stats *wireStats,
	started time.Time,
) {
	if graph != nil {
		for _, resource := range graph.emittedNodes() {
			switch resource.AcquisitionState {
			case "readable":
				result.Metrics.Resources["readable"]++
			case "unreadable":
				result.Metrics.Resources["unreadable"]++
			default:
				result.Metrics.Resources["unknown"]++
			}
		}
	}
	result.Metrics.Resources["discovered"] = result.Metrics.Resources["readable"] +
		result.Metrics.Resources["unreadable"] + result.Metrics.Resources["unknown"]
	if result.Complete {
		result.Metrics.Status = "success"
	} else {
		result.Metrics.Status = "partial"
	}
	result.Metrics.Duration = time.Since(started).Seconds()
	c.copyWireStats(&result.Metrics, stats)
	result.Diagnostics = appendUniqueDiagnostics(
		result.Diagnostics,
		responseCompatibilityDiagnostics(stats)...,
	)
}

func recordResponseCompatibility(stats *wireStats, header http.Header) {
	if stats == nil {
		return
	}
	if stats.responses == nil {
		stats.responses = make(map[string]struct{})
	}
	for name, state := range map[string]string{
		"Content-Type":  responseContentTypeState(header),
		"OData-Version": responseODataVersionState(header),
	} {
		if state == "valid" {
			continue
		}
		stats.responses[name+"\x00"+state] = struct{}{}
	}
}

func responseCompatibilityDiagnostics(stats *wireStats) []string {
	if stats == nil {
		return nil
	}
	var result []string
	for _, name := range []string{"Content-Type", "OData-Version"} {
		for _, state := range []string{"missing", "invalid"} {
			if _, ok := stats.responses[name+"\x00"+state]; !ok {
				continue
			}
			result = append(result, boundedDiagnostic(
				fmt.Sprintf("Redfish compatibility: response %s header is %s", name, state),
			))
		}
	}
	return result
}

func appendUniqueDiagnostics(current []string, values ...string) []string {
	seen := make(map[string]struct{}, len(current)+len(values))
	for _, value := range current {
		seen[value] = struct{}{}
	}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		current = append(current, value)
	}
	return current
}

func (c *protocolClient) rememberCompatibilityDiagnostics(stats *wireStats) {
	diagnostics := responseCompatibilityDiagnostics(stats)
	if len(diagnostics) == 0 {
		return
	}
	c.diagnosticMu.Lock()
	if c.pendingCompatibilityDiag == nil {
		c.pendingCompatibilityDiag = make(map[string]struct{})
	}
	for _, diagnostic := range diagnostics {
		c.pendingCompatibilityDiag[diagnostic] = struct{}{}
	}
	c.diagnosticMu.Unlock()
}

func (c *protocolClient) takeCompatibilityDiagnostics() []string {
	c.diagnosticMu.Lock()
	defer c.diagnosticMu.Unlock()
	var result []string
	for _, name := range []string{"Content-Type", "OData-Version"} {
		for _, state := range []string{"missing", "invalid"} {
			diagnostic := boundedDiagnostic(
				fmt.Sprintf("Redfish compatibility: response %s header is %s", name, state),
			)
			if _, ok := c.pendingCompatibilityDiag[diagnostic]; ok {
				result = append(result, diagnostic)
			}
		}
	}
	c.pendingCompatibilityDiag = nil
	return result
}

func (c *protocolClient) copyWireStats(metrics *cycleMetrics, stats *wireStats) {
	metrics.HTTPRequests["started"] = stats.started
	metrics.HTTPRequests["retried"] = stats.retried
	metrics.HTTPRequests["redirected"] = stats.redirected
	metrics.Operations["successful"] = stats.successful
	metrics.Operations["failed"] = stats.failed
	metrics.ReceivedBytes = stats.received
	maps.Copy(metrics.Failures, stats.failures)
}

func (c *protocolClient) Close(ctx context.Context) error {
	c.authMu.Lock()
	sessions := append([]sessionHandle(nil), c.sessions...)
	if c.sessionURI != "" && c.token != "" && len(sessions) == 0 {
		sessions = append(sessions, sessionHandle{token: c.token, uri: c.sessionURI})
	}
	c.sessionURI = ""
	c.token = ""
	c.sessions = nil
	c.authMode = "none"
	c.authMu.Unlock()

	var result error
	seen := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session.uri == "" || session.token == "" {
			continue
		}
		key := session.uri + "\x00" + session.token
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = errors.Join(result, c.deleteSessionIndependent(ctx, session, nil))
	}
	return result
}

var errSessionUnsupported = errors.New("Redfish session authentication is unsupported")

func (c *protocolClient) initializeSession(
	ctx context.Context,
	root *serviceRootDocument,
	stats *wireStats,
) error {
	if err := c.retirePendingSessions(ctx, stats); err != nil {
		return err
	}
	candidates := nonEmptyStrings(root.Links.Sessions.ODataID)
	tried := make(map[string]struct{})
	var unsupported error
	legacyEligible := false

	try := func(rawURI string) (bool, error) {
		target, err := c.resolveURI(c.root, rawURI, false)
		if err != nil {
			return false, fmt.Errorf("resolve Sessions collection: %w", err)
		}
		key := canonicalResourceURI(target)
		if _, ok := tried[key]; ok {
			return false, nil
		}
		tried[key] = struct{}{}
		err = c.createSession(ctx, target, stats)
		if errors.Is(err, errSessionUnsupported) {
			var status statusError
			if errors.As(err, &status) && status.status == http.StatusMethodNotAllowed {
				legacyEligible = true
			}
			unsupported = errors.Join(unsupported, err)
			return false, nil
		}
		return err == nil, err
	}

	for _, candidate := range candidates {
		if ok, err := try(candidate); err != nil {
			return err
		} else if ok {
			return nil
		}
	}

	if root.SessionService.ODataID != "" {
		sessionsURI, err := c.readSessionServiceSessions(ctx, root.SessionService.ODataID, stats)
		if err != nil {
			if !errors.Is(err, errSessionUnsupported) {
				return err
			}
			unsupported = errors.Join(unsupported, err)
		} else if ok, err := try(sessionsURI); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	if legacyEligible {
		if ok, err := try(legacySessionsURI); err != nil {
			return err
		} else if ok {
			return nil
		}
	}
	if len(tried) == 0 && unsupported == nil {
		return errSessionUnsupported
	}
	return errSessionUnsupported
}

func (c *protocolClient) readSessionServiceSessions(
	ctx context.Context,
	rawURI string,
	stats *wireStats,
) (string, error) {
	target, err := c.resolveURI(c.root, rawURI, false)
	if err != nil {
		return "", fmt.Errorf("resolve SessionService: %w", err)
	}
	response, err := c.do(
		ctx,
		protocolRequest{method: http.MethodGet, target: target, auth: requestAuth{}},
		stats,
		true,
		http.StatusOK,
	)
	if err != nil {
		var status statusError
		if errors.As(err, &status) && sessionUnsupportedStatus(status.status) {
			return "", errSessionUnsupported
		}
		return "", fmt.Errorf("read SessionService: %w", err)
	}
	var service struct {
		ODataID   string      `json:"@odata.id"`
		ODataType string      `json:"@odata.type"`
		ID        string      `json:"Id"`
		Name      string      `json:"Name"`
		Sessions  redfishLink `json:"Sessions"`
	}
	if err := decodeJSON(response, &service); err != nil {
		response.finish(err)
		return "", fmt.Errorf("decode SessionService: %w", err)
	}
	resolved, err := c.resolveURI(response.url, service.ODataID, false)
	if err != nil || !sameResourceIdentity(canonicalResourceURI(resolved), canonicalResourceURI(response.url)) {
		err = errors.New("SessionService identity does not match requested URI")
		response.finish(err)
		return "", err
	}
	if err := validateResourceSchemaType("session_service", service.ODataType); err != nil {
		response.finish(err)
		return "", err
	}
	if strings.TrimSpace(service.ID) == "" || strings.TrimSpace(service.Name) == "" {
		err := errors.New("SessionService has no usable Id or Name")
		response.finish(err)
		return "", err
	}
	if service.Sessions.ODataID == "" {
		err := errors.New("SessionService has no Sessions collection")
		response.finish(err)
		return "", err
	}
	sessionsTarget, err := c.resolveURI(response.url, service.Sessions.ODataID, false)
	if err != nil {
		err = fmt.Errorf("resolve SessionService Sessions collection: %w", err)
		response.finish(err)
		return "", err
	}
	response.finish(nil)
	return sessionsTarget.String(), nil
}

func (c *protocolClient) createSession(
	ctx context.Context,
	target *url.URL,
	stats *wireStats,
) error {
	body, err := json.Marshal(map[string]string{
		"UserName": c.config.Username,
		"Password": c.config.Password,
	})
	if err != nil {
		return fmt.Errorf("encode session request: %w", err)
	}
	response, err := c.do(
		ctx,
		protocolRequest{method: http.MethodPost, target: target, body: body, auth: requestAuth{}},
		stats,
		false,
		http.StatusCreated,
	)
	if err != nil {
		var status statusError
		if errors.As(err, &status) && sessionUnsupportedStatus(status.status) {
			return fmt.Errorf("%w: %w", errSessionUnsupported, err)
		}
		return fmt.Errorf("create Redfish session: %w", err)
	}
	token := response.header.Get("X-Auth-Token")
	location := response.header.Get("Location")
	if len(token) > maxSessionTokenBytes || len(location) > maxURIBytes {
		err := errors.New("create Redfish session: response has an oversized X-Auth-Token or Location")
		response.finish(err)
		return err
	}
	token = strings.TrimSpace(token)
	location = strings.TrimSpace(location)
	if token == "" || location == "" {
		err := errors.New("create Redfish session: response is missing X-Auth-Token or Location")
		response.finish(err)
		return err
	}
	sessionTarget, err := c.resolveURI(target, location, false)
	if err != nil {
		err = fmt.Errorf("create Redfish session: invalid Location: %w", err)
		response.finish(err)
		return err
	}
	handle := sessionHandle{token: token, uri: canonicalResourceURI(sessionTarget)}
	c.recordSession(handle)
	fail := func(err error) error {
		response.finish(err)
		if c.deleteSessionIndependent(ctx, handle, stats) == nil {
			c.forgetSession(handle)
		}
		return err
	}
	var session struct {
		ODataType string `json:"@odata.type"`
		ODataID   string `json:"@odata.id"`
		ID        string `json:"Id"`
		Name      string `json:"Name"`
	}
	if err := decodeJSON(response, &session); err != nil {
		return fail(fmt.Errorf("decode created Redfish session: %w", err))
	}
	if strings.TrimSpace(session.ID) == "" || strings.TrimSpace(session.Name) == "" {
		return fail(errors.New("create Redfish session: body is not a complete Session resource"))
	}
	sessionID, err := c.resolveURI(target, session.ODataID, false)
	if err != nil || !sameResourceIdentity(canonicalResourceURI(sessionID), canonicalResourceURI(sessionTarget)) {
		return fail(errors.New("create Redfish session: body identity does not match Location"))
	}
	if err := validateResourceSchemaType("session", session.ODataType); err != nil {
		return fail(fmt.Errorf("create Redfish session: %w", err))
	}
	response.finish(nil)
	c.activateSession(handle)
	return nil
}

func sessionUnsupportedStatus(status int) bool {
	return status == http.StatusNotFound ||
		status == http.StatusMethodNotAllowed ||
		status == http.StatusNotImplemented
}

func (c *protocolClient) setAuth(mode, token, sessionURI string) {
	c.authMu.Lock()
	c.authMode = mode
	c.token = token
	c.sessionURI = sessionURI
	c.sessionInit = true
	if mode == "session" && token != "" && sessionURI != "" {
		handle := sessionHandle{token: token, uri: sessionURI}
		if !containsSession(c.sessions, handle) {
			c.sessions = append(c.sessions, handle)
		}
	}
	c.authMu.Unlock()
}

func (c *protocolClient) recordSession(handle sessionHandle) {
	c.authMu.Lock()
	if !containsSession(c.sessions, handle) {
		c.sessions = append(c.sessions, handle)
	}
	c.authMu.Unlock()
}

func (c *protocolClient) activateSession(handle sessionHandle) {
	c.setAuth("session", handle.token, handle.uri)
}

func (c *protocolClient) retirePendingSessions(ctx context.Context, stats *wireStats) error {
	c.authMu.RLock()
	sessions := append([]sessionHandle(nil), c.sessions...)
	active := sessionHandle{}
	if c.authMode == "session" {
		active = sessionHandle{token: c.token, uri: c.sessionURI}
	}
	c.authMu.RUnlock()

	var joined error
	for _, session := range sessions {
		if session == active {
			continue
		}
		if err := c.deleteSessionIndependent(ctx, session, stats); err != nil {
			joined = errors.Join(joined, fmt.Errorf("retire unactivated Redfish session: %w", err))
			continue
		}
		c.forgetSession(session)
	}
	return joined
}

func containsSession(sessions []sessionHandle, handle sessionHandle) bool {
	return slices.Contains(sessions, handle)
}

func (c *protocolClient) deleteSession(
	ctx context.Context,
	session sessionHandle,
	stats *wireStats,
) error {
	target, err := c.resolveURI(c.root, session.uri, false)
	if err != nil {
		return nil
	}
	response, err := c.do(
		ctx,
		protocolRequest{method: http.MethodDelete, target: target, auth: requestAuth{token: session.token}},
		stats,
		false,
		http.StatusNoContent,
		http.StatusOK,
		http.StatusNotFound,
		http.StatusGone,
	)
	if response != nil {
		response.finish(err)
	}
	return err
}

func (c *protocolClient) deleteSessionIndependent(
	parent context.Context,
	session sessionHandle,
	stats *wireStats,
) error {
	timeout := min(c.config.Timeout.Duration(), 5*time.Second)
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	base := context.WithoutCancel(parent)
	if deadline, ok := parent.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			timeout = min(timeout, remaining)
		}
	}
	cleanupCtx, cancel := context.WithTimeout(base, timeout)
	defer cancel()
	return c.deleteSession(cleanupCtx, session, stats)
}

func (c *protocolClient) refreshSession(
	ctx context.Context,
	expiredToken string,
	stats *wireStats,
) (requestAuth, error) {
	c.refreshMu.Lock()
	defer c.refreshMu.Unlock()

	current := c.currentAuth(true)
	if current.mode != "session" {
		return requestAuth{}, errors.New("Redfish session authentication is no longer active")
	}
	if current.token != expiredToken {
		return current, nil
	}

	c.authMu.RLock()
	old := sessionHandle{token: c.token, uri: c.sessionURI}
	c.authMu.RUnlock()
	root, err := c.fetchServiceRoot(ctx, false, stats)
	if err != nil {
		return requestAuth{}, fmt.Errorf("re-read ServiceRoot for session recovery: %w", err)
	}
	if err := c.initializeSession(ctx, root, stats); err != nil {
		return requestAuth{}, fmt.Errorf("recreate Redfish session: %w", err)
	}
	// A 401 has invalidated this credential. Deletion is best effort: retaining
	// an expired token as pending cleanup would block the next refresh forever.
	_ = c.deleteSessionIndependent(ctx, old, stats)
	c.forgetSession(old)
	return c.currentAuth(true), nil
}

func (c *protocolClient) forgetSession(session sessionHandle) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	for i, candidate := range c.sessions {
		if candidate == session {
			c.sessions = append(c.sessions[:i], c.sessions[i+1:]...)
			return
		}
	}
}

type requestAuth struct {
	mode     string
	username string
	password string
	token    string
}

type protocolRequest struct {
	method string
	target *url.URL
	body   []byte
	auth   requestAuth
}

func (c *protocolClient) currentAuth(enabled bool) requestAuth {
	if !enabled {
		return requestAuth{}
	}
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	switch c.authMode {
	case "basic":
		return requestAuth{mode: "basic", username: c.config.Username, password: c.config.Password}
	case "session":
		return requestAuth{mode: "session", token: c.token}
	default:
		return requestAuth{}
	}
}

func (c *protocolClient) fetchServiceRoot(ctx context.Context, authenticated bool, stats *wireStats) (*serviceRootDocument, error) {
	response, err := c.do(
		ctx,
		protocolRequest{method: http.MethodGet, target: c.root, auth: c.currentAuth(authenticated)},
		stats,
		true,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	var root serviceRootDocument
	if err := decodeJSON(response, &root); err != nil {
		response.finish(err)
		return nil, fmt.Errorf("decode ServiceRoot: %w", err)
	}
	if err := decodeJSON(response, &root.Raw); err != nil {
		response.finish(err)
		return nil, fmt.Errorf("decode raw ServiceRoot: %w", err)
	}
	if err := validateRequiredResourceProperties("service", root.Raw); err != nil {
		response.finish(err)
		return nil, err
	}
	resolvedID, err := c.resolveURI(response.url, root.ODataID, false)
	if err != nil ||
		!sameResourceIdentity(canonicalResourceURI(resolvedID), canonicalResourceURI(response.url)) {
		err = errors.New("ServiceRoot identity does not match requested URI")
		response.finish(err)
		return nil, err
	}
	if err := validateResourceSchemaType("service", root.ODataType); err != nil {
		response.finish(err)
		return nil, err
	}
	if !validRedfishVersion(root.RedfishVersion) {
		err := errors.New("ServiceRoot has no valid RedfishVersion")
		response.finish(err)
		return nil, err
	}
	root.Response = metadataForResponse(response)
	response.finish(nil)
	c.serviceMetaMu.Lock()
	c.serviceName = root.Name
	c.redfishVersion = root.RedfishVersion
	c.serviceMetaMu.Unlock()
	if root.ProtocolFeaturesSupported.MultipleHTTPRequests != nil &&
		!*root.ProtocolFeaturesSupported.MultipleHTTPRequests {
		c.setRequestLimit(1)
	} else {
		c.setRequestLimit(c.config.MaxConcurrentRequests)
	}
	c.setExpansionValue(expansionValue(&root))
	return &root, nil
}

type baseResource struct {
	Kind               string
	URI                string
	Doc                genericResource
	Data               map[string]any
	Response           responseMetadata
	AcquisitionState   string
	ErrorClass         string
	MembershipComplete bool
}

func (c *protocolClient) fetchBaseResources(
	ctx context.Context,
	root *serviceRootDocument,
	stats *wireStats,
) ([]baseResource, map[string]bool, bool, error) {
	collections := []struct {
		kind string
		link redfishLink
	}{
		{"system", root.Systems},
		{"chassis", root.Chassis},
		{"manager", root.Managers},
	}
	var resources []baseResource
	membership := make(map[string]bool, len(collections))
	complete := true
	var joined error
	for _, collection := range collections {
		if collection.link.ODataID == "" {
			membership[collection.kind] = true
			c.replaceBaseMembership(collection.kind, nil)
			continue
		}
		members, ok, err := c.fetchCollectionMembers(ctx, collection.link.ODataID, collection.kind, stats)
		if err != nil {
			membership[collection.kind] = false
			complete = false
			joined = errors.Join(joined, fmt.Errorf("%s collection: %w", collection.kind, err))
			current, memberErr := c.fetchBaseMembers(
				ctx,
				collection.kind,
				members,
				stats,
				false,
			)
			joined = errors.Join(joined, memberErr)
			for _, resource := range c.incompleteBaseMembership(collection.kind, current) {
				resources = append(resources, resource)
			}
			continue
		}
		membership[collection.kind] = ok
		complete = complete && ok
		current, memberErr := c.fetchBaseMembers(
			ctx,
			collection.kind,
			members,
			stats,
			true,
		)
		if memberErr != nil {
			complete = false
			joined = errors.Join(joined, memberErr)
		}
		c.replaceBaseMembership(collection.kind, current)
		for _, resource := range current {
			resources = append(resources, resource)
		}
	}
	return resources, membership, complete, joined
}

func (c *protocolClient) fetchBaseMembers(
	ctx context.Context,
	kind string,
	members []collectionMember,
	stats *wireStats,
	membershipComplete bool,
) ([]baseResource, error) {
	current := make([]baseResource, len(members))
	var failures boundedErrorAccumulator
	completed := true
	for index := range members {
		if err := contextError(ctx); err != nil {
			failures.Add(err)
			completed = false
			break
		}
		member := members[index]
		resource, err := c.fetchBaseMember(ctx, kind, member, stats)
		if err != nil {
			failures.Add(fmt.Errorf("%s resource: %w", kind, err))
			resource = c.unreadableBaseResource(kind, member.Ref.ODataID, classifyError(err))
			if isCallerContextError(err) {
				completed = false
			}
		}
		resource.MembershipComplete = membershipComplete
		current[index] = resource
		if !completed {
			break
		}
	}
	if !completed {
		for index := range current {
			if current[index].URI != "" {
				continue
			}
			resource := c.unreadableBaseResource(
				kind,
				members[index].Ref.ODataID,
				"timeout",
			)
			resource.AcquisitionState = "unknown"
			resource.MembershipComplete = membershipComplete
			current[index] = resource
		}
	}
	return current, failures.Err()
}

func (c *protocolClient) fetchBaseMember(
	ctx context.Context,
	kind string,
	member collectionMember,
	stats *wireStats,
) (baseResource, error) {
	if member.Data == nil || len(member.Raw) == 0 {
		return c.fetchBaseResource(ctx, kind, member.Ref, stats)
	}
	var doc genericResource
	if err := json.Unmarshal(member.Raw, &doc); err != nil {
		return baseResource{}, fmt.Errorf("decode expanded %s resource: %w", kind, err)
	}
	if err := validateRequiredResourceProperties(kind, member.Data); err != nil {
		return baseResource{}, err
	}
	return baseResource{
		Kind:               kind,
		URI:                member.Ref.ODataID,
		Doc:                doc,
		Data:               cloneJSONMap(member.Data),
		Response:           member.Response,
		AcquisitionState:   "readable",
		MembershipComplete: true,
	}, nil
}

func (c *protocolClient) fetchBaseResource(
	ctx context.Context,
	kind string,
	ref redfishLink,
	stats *wireStats,
) (baseResource, error) {
	target, err := c.resolveURI(c.root, ref.ODataID, false)
	if err != nil {
		return baseResource{}, err
	}
	response, err := c.do(
		ctx,
		protocolRequest{method: http.MethodGet, target: target, auth: c.currentAuth(true)},
		stats,
		true,
		http.StatusOK,
	)
	if err != nil {
		return baseResource{}, err
	}
	var doc genericResource
	if err := decodeJSON(response, &doc); err != nil {
		response.finish(err)
		return baseResource{}, err
	}
	var data map[string]any
	if err := decodeJSON(response, &data); err != nil {
		response.finish(err)
		return baseResource{}, err
	}
	if err := validateRequiredResourceProperties(kind, data); err != nil {
		response.finish(err)
		return baseResource{}, err
	}
	resolvedID, err := c.resolveURI(response.url, doc.ODataID, false)
	if err != nil ||
		!sameResourceIdentity(canonicalResourceURI(resolvedID), canonicalResourceURI(response.url)) {
		err = errors.New("resource identity does not match final response URI")
		response.finish(err)
		return baseResource{}, err
	}
	rawType, _ := stringValue(data["@odata.type"])
	if err := validateResourceSchemaType(kind, rawType); err != nil {
		response.finish(err)
		return baseResource{}, err
	}
	response.finish(nil)
	return baseResource{
		Kind:               kind,
		URI:                canonicalResourceURI(response.url),
		Doc:                doc,
		Data:               data,
		Response:           metadataForResponse(response),
		AcquisitionState:   "readable",
		MembershipComplete: true,
	}, nil
}

func (c *protocolClient) replaceBaseMembership(kind string, resources []baseResource) {
	c.baseMu.Lock()
	c.baseMembership[kind] = snapshotBaseResources(resources)
	c.baseMu.Unlock()
}

func (c *protocolClient) incompleteBaseMembership(
	kind string,
	current []baseResource,
) []baseResource {
	c.baseMu.Lock()
	defer c.baseMu.Unlock()
	resources := append([]baseResource(nil), current...)
	present := make(map[string]struct{}, len(current))
	for i := range resources {
		present[resources[i].URI] = struct{}{}
		resources[i].MembershipComplete = false
	}
	for _, retained := range c.baseMembership[kind] {
		if _, ok := present[retained.URI]; ok {
			continue
		}
		retained.MembershipComplete = false
		retained.AcquisitionState = "unknown"
		retained.ErrorClass = "protocol"
		clearCurrentBaseResource(&retained)
		resources = append(resources, retained)
	}
	c.baseMembership[kind] = snapshotBaseResources(resources)
	return resources
}

func (c *protocolClient) unreadableBaseResource(kind, uri, class string) baseResource {
	resource := baseResource{
		Kind:               kind,
		URI:                uri,
		AcquisitionState:   "unreadable",
		ErrorClass:         class,
		MembershipComplete: true,
	}
	c.baseMu.Lock()
	for _, previous := range c.baseMembership[kind] {
		if previous.URI == uri {
			resource.Doc = previous.Doc
			break
		}
	}
	c.baseMu.Unlock()
	clearCurrentBaseResource(&resource)
	return resource
}

func clearCurrentBaseResource(resource *baseResource) {
	resource.Doc.Status = genericStatus{}
	resource.Doc.PowerState = ""
	resource.Doc.FailurePredicted = nil
	resource.Data = nil
}

// Membership continuity needs source identities, never previous measurements.
func snapshotBaseResources(resources []baseResource) []baseResource {
	result := append([]baseResource(nil), resources...)
	for i := range result {
		clearCurrentBaseResource(&result[i])
		result[i].Response = responseMetadata{}
	}
	return result
}

func (c *protocolClient) fetchCollection(
	ctx context.Context,
	ref string,
	stats *wireStats,
) ([]redfishLink, bool, error) {
	members, complete, err := c.fetchCollectionMembers(ctx, ref, "", stats)
	return collectionMemberLinks(members), complete, err
}

func (c *protocolClient) fetchCollectionMembers(
	ctx context.Context,
	ref string,
	kind string,
	stats *wireStats,
) ([]collectionMember, bool, error) {
	target, err := c.resolveURI(c.root, ref, false)
	if err != nil {
		return nil, false, err
	}
	return c.fetchCollectionMembersAt(ctx, target, nil, kind, stats)
}

func (c *protocolClient) fetchCollectionMembersAt(
	ctx context.Context,
	target *url.URL,
	first *responseData,
	kind string,
	stats *wireStats,
) ([]collectionMember, bool, error) {
	identity := canonicalResourceURI(target)
	c.markKnownCollection(identity)
	if first == nil && kind != "" {
		if expand := c.collectionExpansion(identity); expand != "" {
			expanded := *target
			query := make(url.Values, 1)
			query.Set("$expand", expand)
			expanded.RawQuery = query.Encode()
			members, complete, err := c.fetchCollectionMemberPages(
				ctx,
				&expanded,
				nil,
				stats,
				kind,
				true,
			)
			if err == nil && complete {
				return members, true, nil
			}
			if persistentCollectionExpansionFailure(err) {
				c.disableCollectionExpansion(identity, expand)
			}
			if isCallerContextError(err) {
				return members, false, err
			}
		}
	}
	return c.fetchCollectionMemberPages(
		ctx,
		target,
		first,
		stats,
		kind,
		false,
	)
}

func persistentCollectionExpansionFailure(err error) bool {
	if err == nil || isCallerContextError(err) {
		return false
	}
	var status statusError
	if errors.As(err, &status) {
		switch status.status {
		case http.StatusBadRequest,
			http.StatusNotFound,
			http.StatusMethodNotAllowed,
			http.StatusNotAcceptable,
			http.StatusRequestURITooLong,
			http.StatusUnsupportedMediaType,
			http.StatusUnprocessableEntity,
			http.StatusNotImplemented:
			return true
		default:
			return false
		}
	}
	switch classifyError(err) {
	case "auth", "tls", "transport", "timeout", "limit":
		return false
	default:
		return true
	}
}

func (c *protocolClient) fetchCollectionMemberPages(
	ctx context.Context,
	target *url.URL,
	first *responseData,
	stats *wireStats,
	expectedKind string,
	requireExpanded bool,
) ([]collectionMember, bool, error) {
	progress := collectionProgress{
		CollectionIdentity: canonicalResourceURI(target),
		ExpectedCount:      -1,
		SeenPages:          make(map[string]struct{}),
		SeenMembers:        make(map[string]struct{}),
	}
	fail := func(response *responseData, err error) ([]collectionMember, bool, error) {
		if response != nil {
			response.finish(err)
		}
		return progress.Members, false, err
	}

	for {
		if err := contextError(ctx); err != nil {
			return fail(nil, err)
		}
		pageKey := target.String()
		if _, ok := progress.SeenPages[pageKey]; ok {
			return fail(nil, errors.New("collection pagination loop"))
		}
		response := first
		first = nil
		if response == nil {
			var err error
			response, err = c.do(
				ctx,
				protocolRequest{method: http.MethodGet, target: target, auth: c.currentAuth(true)},
				stats,
				true,
				http.StatusOK,
			)
			if err != nil {
				return fail(nil, err)
			}
		}
		var page collectionPage
		if err := decodeJSON(response, &page); err != nil {
			return fail(response, err)
		}
		if len(progress.SeenPages) == 0 {
			progress.CollectionIdentity = canonicalResourceURI(response.url)
		}
		resolvedPageID, err := c.resolveURI(response.url, page.ODataID, false)
		if err != nil ||
			!sameResourceIdentity(canonicalResourceURI(resolvedPageID), progress.CollectionIdentity) {
			err = errors.New("collection identity does not match the requested collection")
			return fail(response, err)
		}
		if err := validateCollectionSchemaType(page.ODataType, expectedKind); err != nil {
			return fail(response, err)
		}
		if page.Count == nil || *page.Count < 0 {
			err := errors.New("collection page has no valid @odata.count")
			return fail(response, err)
		}
		if progress.ExpectedCount < 0 {
			progress.ExpectedCount = *page.Count
		} else if progress.ExpectedCount != *page.Count {
			err := errors.New("collection @odata.count changed between pages")
			return fail(response, err)
		}
		if len(page.Members) == 0 || bytes.Equal(bytes.TrimSpace(page.Members), []byte("null")) {
			err := errors.New("collection page has no Members array")
			return fail(response, err)
		}
		var rawMembers []json.RawMessage
		if err := json.Unmarshal(page.Members, &rawMembers); err != nil {
			return fail(response, errors.New("collection Members is not an array"))
		}
		for _, rawMember := range rawMembers {
			var data map[string]any
			if err := decodeJSONBytes(rawMember, &data); err != nil || data == nil {
				err := errors.New("collection member is not an object")
				if requireExpanded {
					return fail(response, err)
				}
				progress.recordInvalidMember(err)
				continue
			}
			rawURI, ok := stringValue(data["@odata.id"])
			if !ok {
				err := errors.New("collection member has no @odata.id")
				if requireExpanded {
					return fail(response, err)
				}
				progress.recordInvalidMember(err)
				continue
			}
			memberTarget, err := c.resolveURI(response.url, rawURI, false)
			if err != nil {
				if requireExpanded {
					return fail(response, err)
				}
				progress.recordInvalidMember(err)
				continue
			}
			key := canonicalResourceURI(memberTarget)
			if _, ok := progress.SeenMembers[key]; ok {
				err := errors.New("collection contains duplicate member identity")
				if requireExpanded {
					return fail(response, err)
				}
				progress.recordInvalidMember(err)
				continue
			}
			member := collectionMember{Ref: redfishLink{ODataID: key}}
			if requireExpanded {
				encoded, err := validateExpandedCollectionMember(expectedKind, data, key, response.url)
				if err != nil {
					return fail(response, err)
				}
				member.Data = data
				member.Raw = encoded
				member.Response = metadataForResponse(response)
			}
			progress.SeenMembers[key] = struct{}{}
			progress.Members = append(progress.Members, member)
		}
		progress.SeenPages[pageKey] = struct{}{}
		if page.NextLink == "" {
			var completionErr error
			if progress.InvalidMembers > 0 {
				completionErr = fmt.Errorf(
					"collection skipped %d invalid or duplicate members: %s",
					progress.InvalidMembers,
					progress.FirstMemberError,
				)
			}
			if len(progress.Members) != progress.ExpectedCount {
				completionErr = errors.Join(completionErr, fmt.Errorf(
					"collection has %d unique members, advertised %d",
					len(progress.Members),
					progress.ExpectedCount,
				))
			}
			if completionErr != nil {
				response.finish(completionErr)
				return progress.Members, false, completionErr
			}
			response.finish(nil)
			return progress.Members, true, nil
		}
		next, err := c.resolveURI(response.url, page.NextLink, true)
		if err != nil {
			return fail(response, err)
		}
		response.finish(nil)
		target = next
	}
}

func (p *collectionProgress) recordInvalidMember(err error) {
	p.InvalidMembers++
	if p.FirstMemberError == "" {
		p.FirstMemberError = err.Error()
	}
}

func collectionMemberLinks(members []collectionMember) []redfishLink {
	result := make([]redfishLink, len(members))
	for i := range members {
		result[i] = members[i].Ref
	}
	return result
}

func isCallerContextError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func validateExpandedCollectionMember(
	kind string,
	data map[string]any,
	identity string,
	base *url.URL,
) ([]byte, error) {
	if kind == "" {
		return nil, errors.New("expanded collection member has no expected resource kind")
	}
	rawID, ok := stringValue(data["@odata.id"])
	if !ok {
		return nil, errors.New("expanded collection member has no identity")
	}
	resolved, err := resolveRedfishURI(
		(&url.URL{Scheme: base.Scheme, Host: base.Host}).String(),
		base,
		rawID,
		uriResource,
	)
	if err != nil || !sameResourceIdentity(canonicalResourceURI(resolved), identity) {
		return nil, errors.New("expanded collection member identity does not match its link")
	}
	rawType, ok := stringValue(data["@odata.type"])
	if !ok {
		return nil, errors.New("expanded collection member has no schema type")
	}
	if err := validateResourceSchemaType(kind, rawType); err != nil {
		return nil, err
	}
	if err := validateRequiredResourceProperties(kind, data); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, errors.New("encode expanded collection member")
	}
	return encoded, nil
}

func expansionValue(root *serviceRootDocument) string {
	if root == nil {
		return ""
	}
	expand := root.ProtocolFeaturesSupported.ExpandQuery
	switch {
	case expand.NoLinks:
		return "."
	case expand.Links:
		return "~"
	case expand.ExpandAll:
		return "*"
	default:
		return ""
	}
}

func (c *protocolClient) setExpansionValue(value string) {
	c.collectionMu.Lock()
	c.ensureCollectionStateLocked()
	c.expansionValue = value
	c.collectionMu.Unlock()
}

func (c *protocolClient) collectionExpansion(identity string) string {
	c.collectionMu.Lock()
	defer c.collectionMu.Unlock()
	c.ensureCollectionStateLocked()
	if c.expansionValue == "" {
		return ""
	}
	key := identity + "\x00" + c.expansionValue
	if _, disabled := c.expansionDisabled[key]; disabled {
		return ""
	}
	return c.expansionValue
}

func (c *protocolClient) disableCollectionExpansion(identity, value string) {
	c.collectionMu.Lock()
	c.ensureCollectionStateLocked()
	c.expansionDisabled[identity+"\x00"+value] = struct{}{}
	c.expansionFallbackSeen = true
	c.collectionMu.Unlock()
}

func (c *protocolClient) takeExpansionFallbackDiagnostic() string {
	c.collectionMu.Lock()
	defer c.collectionMu.Unlock()
	if !c.expansionFallbackSeen {
		return ""
	}
	c.expansionFallbackSeen = false
	return "Redfish compatibility: advertised collection query expansion was rejected; using ordinary member links"
}

func (c *protocolClient) markKnownCollection(identity string) {
	c.collectionMu.Lock()
	c.ensureCollectionStateLocked()
	c.knownCollections[identity] = struct{}{}
	c.collectionMu.Unlock()
}

func (c *protocolClient) isKnownCollection(identity string) bool {
	c.collectionMu.Lock()
	defer c.collectionMu.Unlock()
	c.ensureCollectionStateLocked()
	_, ok := c.knownCollections[identity]
	return ok
}

func (c *protocolClient) ensureCollectionStateLocked() {
	if c.knownCollections == nil {
		c.knownCollections = make(map[string]struct{})
	}
	if c.expansionDisabled == nil {
		c.expansionDisabled = make(map[string]struct{})
	}
}

func (c *protocolClient) do(
	ctx context.Context,
	request protocolRequest,
	stats *wireStats,
	retry bool,
	accepted ...int,
) (*responseData, error) {
	allowed := make(map[int]struct{}, len(accepted))
	for _, status := range accepted {
		allowed[status] = struct{}{}
	}
	retries := 0
	if retry && request.method == http.MethodGet && c.config.Retries != nil {
		retries = *c.config.Retries
	}
	sessionReplayed := false
	for attempt := 0; ; {
		response, err := c.doRedirectChain(ctx, request, stats)
		if err == nil {
			if _, ok := allowed[response.status]; ok {
				return response, nil
			}
			if response.status == http.StatusUnauthorized && request.method == http.MethodGet &&
				request.auth.mode == "session" && !sessionReplayed {
				replacement, refreshErr := c.refreshSession(ctx, request.auth.token, stats)
				if refreshErr != nil {
					recordWireFailure(stats, "auth")
					return nil, refreshErr
				}
				sessionReplayed = true
				request.auth = replacement
				if stats != nil {
					stats.retried++
				}
				continue
			}
			err = statusError{
				status: response.status,
				class:  classifyHTTPStatus(response.status),
				path:   response.url.EscapedPath(),
			}
			if !retryableStatus(response.status) {
				recordWireFailure(stats, classifyHTTPStatus(response.status))
				return nil, err
			}
		} else if !retryableTransport(err) {
			recordWireFailure(stats, classifyError(err))
			return nil, err
		}
		if attempt >= retries {
			recordWireFailure(stats, classifyError(err))
			return nil, err
		}
		delay := retryDelay(attempt, response)
		if delay > 0 {
			if sleepErr := sleepContext(ctx, delay); sleepErr != nil {
				recordWireFailure(stats, classifyError(sleepErr))
				return nil, sleepErr
			}
		}
		attempt++
		if stats != nil {
			stats.retried++
		}
	}
}

func recordWireFailure(stats *wireStats, class string) {
	if stats == nil {
		return
	}
	stats.failed++
	if stats.failures == nil {
		stats.failures = make(map[string]int)
	}
	stats.failures[class]++
}

func retryDelay(attempt int, response *responseData) time.Duration {
	if response != nil {
		if delay := retryAfter(response.header); delay > 0 {
			return delay
		}
	}
	const initial = 100 * time.Millisecond
	delay := initial << min(attempt, 4)
	return min(delay, time.Second)
}

func (c *protocolClient) doRedirectChain(
	ctx context.Context,
	request protocolRequest,
	stats *wireStats,
) (*responseData, error) {
	seen := make(map[string]struct{}, maxRedirects+1)
	for redirects := 0; ; redirects++ {
		key := request.target.String()
		if _, ok := seen[key]; ok {
			return nil, errors.New("Redfish redirect loop")
		}
		seen[key] = struct{}{}
		response, err := c.doOnce(ctx, request, stats)
		if err != nil {
			return nil, err
		}
		if response.status < 300 || response.status > 399 {
			return response, nil
		}
		if redirects >= maxRedirects {
			return nil, errors.New("Redfish redirect limit exceeded")
		}
		if request.method != http.MethodGet && request.method != http.MethodHead {
			return nil, errors.New("Redfish session request redirect is not allowed")
		}
		location := response.header.Get("Location")
		if location == "" {
			return nil, errors.New("Redfish redirect has no Location")
		}
		next, err := c.resolveURI(request.target, location, request.target.RawQuery != "")
		if err != nil {
			return nil, fmt.Errorf("reject Redfish redirect: %w", err)
		}
		if next.RawQuery != request.target.RawQuery {
			return nil, errors.New("Redfish redirect changed an authorized query")
		}
		if stats != nil {
			stats.redirected++
		}
		request.target = next
	}
}

func (c *protocolClient) doOnce(
	ctx context.Context,
	spec protocolRequest,
	stats *wireStats,
) (*responseData, error) {
	sem, err := c.acquireRequest(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseRequest(sem)

	var reader io.Reader
	if len(spec.body) > 0 {
		reader = bytes.NewReader(spec.body)
	}
	request, err := http.NewRequestWithContext(ctx, spec.method, spec.target.String(), reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("OData-Version", "4.0")
	if len(spec.body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	switch spec.auth.mode {
	case "basic":
		request.SetBasicAuth(spec.auth.username, spec.auth.password)
	case "session":
		if spec.auth.token == "" {
			return nil, errors.New("Redfish session token is unavailable")
		}
		request.Header.Set("X-Auth-Token", spec.auth.token)
	}
	if spec.auth.token != "" && spec.auth.mode == "" {
		request.Header.Set("X-Auth-Token", spec.auth.token)
	}
	if stats != nil {
		stats.started++
	}
	startedAt := time.Now()
	response, err := c.http.Do(request)
	if err != nil {
		return nil, sanitizeTransportError(err)
	}
	defer response.Body.Close()
	limit := int64(maxResponseBodyBytes)
	if response.StatusCode >= 400 {
		limit = maxErrorBodyBytes
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	finishedAt := time.Now()
	if stats != nil {
		stats.received += int64(len(payload))
	}
	if err != nil {
		return nil, sanitizeTransportError(err)
	}
	if int64(len(payload)) > limit {
		return nil, errors.New("Redfish response exceeds the internal body limit")
	}
	return &responseData{
		status:     response.StatusCode,
		header:     response.Header.Clone(),
		body:       payload,
		url:        response.Request.URL,
		startedAt:  startedAt,
		finishedAt: finishedAt,
		stats:      stats,
	}, nil
}

func (c *protocolClient) setRequestLimit(limit int) {
	if limit < 1 {
		limit = 1
	}
	c.semMu.Lock()
	if cap(c.sem) != limit {
		c.sem = make(chan struct{}, limit)
	}
	c.semMu.Unlock()
}

func (c *protocolClient) acquireRequest(ctx context.Context) (chan struct{}, error) {
	c.semMu.RLock()
	sem := c.sem
	c.semMu.RUnlock()
	select {
	case sem <- struct{}{}:
		return sem, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func releaseRequest(sem chan struct{}) {
	<-sem
}

func (c *protocolClient) resolveURI(base *url.URL, raw string, allowQuery bool) (*url.URL, error) {
	mode := uriResource
	if allowQuery {
		mode = uriOpaquePage
	}
	return resolveRedfishURI(c.origin, base, raw, mode)
}

func canonicalResourceURI(target *url.URL) string {
	copy := *target
	copy.RawQuery = ""
	copy.Fragment = ""
	return copy.EscapedPath()
}

func decodeJSON(response *responseData, target any) error {
	if response == nil {
		return errors.New("nil Redfish response")
	}
	return decodeJSONBytes(response.body, target)
}

func decodeJSONBytes(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("Redfish response contains trailing JSON data")
	}
	return nil
}

func metadataForResponse(response *responseData) responseMetadata {
	if response == nil {
		return responseMetadata{}
	}
	return responseMetadata{
		ContentTypeState:  responseContentTypeState(response.header),
		ODataVersionState: responseODataVersionState(response.header),
		StartedAt:         response.startedAt,
		FinishedAt:        response.finishedAt,
	}
}

func responseContentTypeState(header http.Header) string {
	value := header.Get("Content-Type")
	if len(value) > maxContentTypeBytes {
		return "invalid"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "missing"
	}
	mediaType, params, err := mime.ParseMediaType(value)
	if err != nil || !strings.EqualFold(mediaType, "application/json") {
		return "invalid"
	}
	if charset := strings.TrimSpace(params["charset"]); charset != "" &&
		!strings.EqualFold(charset, "utf-8") {
		return "invalid"
	}
	return "valid"
}

func responseODataVersionState(header http.Header) string {
	value := header.Get("OData-Version")
	if len(value) > maxODataVersionBytes {
		return "invalid"
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "missing"
	}
	if value != "4.0" {
		return "invalid"
	}
	return "valid"
}

func retryableStatus(status int) bool {
	return status == http.StatusRequestTimeout ||
		status == http.StatusTooManyRequests ||
		status >= 500
}

func retryAfter(header http.Header) time.Duration {
	value := header.Get("Retry-After")
	if len(value) > maxRetryAfterBytes {
		return 0
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	seconds, err := strconv.ParseUint(value, 10, 64)
	if err == nil && seconds > 0 {
		maxSeconds := uint64(maxRetryAfter / time.Second)
		if seconds >= maxSeconds {
			return maxRetryAfter
		}
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		return min(max(time.Until(when), 0), maxRetryAfter)
	}
	return 0
}

func retryableTransport(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Temporary() ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE)
}

func sanitizeTransportError(err error) error {
	if err == nil {
		return nil
	}
	var tlsErr tls.RecordHeaderError
	if errors.As(err, &tlsErr) {
		return errors.New("Redfish TLS protocol error")
	}
	var verificationError *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameError x509.HostnameError
	if errors.As(err, &verificationError) ||
		errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostnameError) {
		return errors.New("Redfish TLS certificate verification failed")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return transportError{
			timeout: netErr.Timeout(),
			temporary: netErr.Temporary() ||
				errors.Is(err, syscall.ECONNRESET) ||
				errors.Is(err, syscall.EPIPE),
		}
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return transportError{temporary: true}
	}
	return transportError{}
}

func classifyHTTPStatus(status int) string {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		return "auth"
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return "timeout"
	default:
		return "protocol"
	}
}

func classifyError(err error) string {
	var status statusError
	if errors.As(err, &status) {
		return status.class
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "timeout"
	}
	var tlsErr tls.RecordHeaderError
	lower := strings.ToLower(err.Error())
	if errors.As(err, &tlsErr) || strings.Contains(lower, "certificate") || strings.Contains(lower, "tls") {
		return "tls"
	}
	if strings.Contains(lower, "limit") {
		return "limit"
	}
	if errors.As(err, &netErr) {
		return "transport"
	}
	return "protocol"
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func stringAt(data map[string]any, path string) string {
	value, _ := stringValueAt(data, path)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
