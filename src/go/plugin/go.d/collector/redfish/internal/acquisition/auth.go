// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/stmcginnis/gofish"
)

func (c *Client) initializeAuthentication(ctx context.Context, stats *wireStats) error {
	if c.sdk != nil {
		return nil
	}
	client, mode, err := c.connectAuthenticated(ctx, stats)
	if err == nil {
		c.sdk, c.authMode = client.WithContext(context.Background()), mode
	}
	return err
}

func (c *connection) connectAuthenticated(ctx context.Context, stats *wireStats) (*gofish.APIClient, string, error) {
	cfg := c.sdkConfig()
	if c.config.AuthMethod != "none" {
		cfg.Username, cfg.Password = c.config.Username, c.config.Password
		cfg.BasicAuth = c.config.AuthMethod == "basic"
	}
	client, root, err := c.connectSDK(ctx, cfg, stats)
	mode := c.config.AuthMethod
	if mode == "auto" {
		mode = "session"
		if err != nil && sessionUnsupported(root, err) {
			cfg.BasicAuth = true
			client, _, err = c.connectSDK(ctx, cfg, stats)
			mode = "basic"
		}
	}
	return client, mode, err
}

func sessionUnsupported(root *responseData, err error) bool {
	if isCallerContextError(err) {
		return false
	}
	var status statusError
	if errors.As(err, &status) {
		return status.status == http.StatusNotFound || status.status == http.StatusMethodNotAllowed ||
			status.status == http.StatusNotImplemented
	}
	// gofish requires the standard ServiceRoot Links.Sessions link. A service
	// without it can still be monitored with Basic authentication.
	var data map[string]any
	return root != nil && root.status == http.StatusOK && decodeJSON(root, &data) == nil &&
		linkAt(data, "Links.Sessions") == ""
}

func (c *Client) Close() {
	c.closeSession(context.Background())
}

func (c *Client) closeSession(ctx context.Context) {
	client := c.sdk
	c.sdk, c.authMode = nil, ""
	retireSession(ctx, client)
}

func retireSession(ctx context.Context, client *gofish.APIClient) {
	if client == nil {
		return
	}
	session, err := client.GetSession()
	if err != nil {
		return
	}
	// Recovery shares the collection budget; final cleanup has its own cap.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	// Logout replaces an already-canceled context with Background. Delete through
	// the SDK directly so retirement cannot extend a canceled collection.
	_ = client.WithContext(ctx).Service.DeleteSession(session.ID)
}
