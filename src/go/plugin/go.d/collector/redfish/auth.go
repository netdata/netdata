// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"
)

type sessionHandle struct {
	token string
	uri   string
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
	return c.authInitialized
}

func (c *protocolClient) selectedAuthenticationMethod() string {
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	if !c.authInitialized {
		return ""
	}
	return c.authMode
}

func (c *protocolClient) Close(ctx context.Context) error {
	c.authMu.Lock()
	sessions := append([]sessionHandle(nil), c.sessions...)
	if c.sessionURI != "" && c.token != "" && len(sessions) == 0 {
		sessions = append(sessions, sessionHandle{
			token: c.token,
			uri:   c.sessionURI,
		})
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
		protocolRequest{
			method: http.MethodGet,
			target: target,
			auth:   requestAuth{},
		},
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
		protocolRequest{
			method: http.MethodPost,
			target: target,
			body:   body,
			auth:   requestAuth{},
		},
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
	handle := sessionHandle{
		token: token,
		uri:   canonicalResourceURI(sessionTarget),
	}
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
	c.authInitialized = true
	if mode == "session" && token != "" && sessionURI != "" {
		handle := sessionHandle{
			token: token,
			uri:   sessionURI,
		}
		if !slices.Contains(c.sessions, handle) {
			c.sessions = append(c.sessions, handle)
		}
	}
	c.authMu.Unlock()
}

func (c *protocolClient) recordSession(handle sessionHandle) {
	c.authMu.Lock()
	if !slices.Contains(c.sessions, handle) {
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
		active = sessionHandle{
			token: c.token,
			uri:   c.sessionURI,
		}
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
		protocolRequest{
			method: http.MethodDelete,
			target: target,
			auth: requestAuth{
				token: session.token,
			},
		},
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
	old := sessionHandle{
		token: c.token,
		uri:   c.sessionURI,
	}
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

func (c *protocolClient) currentAuth(enabled bool) requestAuth {
	if !enabled {
		return requestAuth{}
	}
	c.authMu.RLock()
	defer c.authMu.RUnlock()
	switch c.authMode {
	case "basic":
		return requestAuth{
			mode:     "basic",
			username: c.config.Username,
			password: c.config.Password,
		}
	case "session":
		return requestAuth{
			mode:  "session",
			token: c.token,
		}
	default:
		return requestAuth{}
	}
}

const (
	maxSessionTokenBytes = 8192
	legacySessionsURI    = "/redfish/v1/Sessions"
)
