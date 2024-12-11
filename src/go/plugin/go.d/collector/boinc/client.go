// SPDX-License-Identifier: GPL-3.0-or-later

package boinc

import (
	"bytes"
	"crypto/md5"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

// Based on: https://github.com/vorot93/boinc-client-rest-server/tree/master

type boincConn interface {
	connect() error
	disconnect()
	authenticate() error
	getResults() ([]boincReplyResult, error)
}

func newBoincConn(conf Config, log *logger.Logger) boincConn {
	return &boincClient{
		Logger: log,

		password: conf.Password,
		conn: socket.New(socket.Config{
			Address: conf.Address,
			Timeout: conf.Timeout.Duration(),
		})}
}

type boincClient struct {
	*logger.Logger
	password string
	conn     socket.Client
}

func (c *boincClient) connect() error {
	return c.conn.Connect()
}

func (c *boincClient) disconnect() {
	_ = c.conn.Disconnect()
}

func (c *boincClient) authenticate() error {
	// https://boinc.berkeley.edu/trac/wiki/GuiRpcProtocol#Authentication

	req := &boincRequest{
		Auth1: &struct{}{},
	}

	resp, err := c.send(req)
	if err != nil {
		return err
	}
	if resp.Nonce == nil {
		return errors.New("auth1: empty nonce")
	}

	req = &boincRequest{
		Auth2: &boincRequestAuthNonce{Hash: makeNonceMD5(*resp.Nonce, c.password)},
	}

	resp, err = c.send(req)
	if err != nil {
		return err
	}
	if resp.Unauthorized != nil || resp.Authorized == nil {
		return errors.New("auth2: unauthorized")
	}

	return nil
}

func (c *boincClient) getResults() ([]boincReplyResult, error) {
	req := &boincRequest{
		GetResults: &boincRequestGetResults{},
	}

	resp, err := c.send(req)
	if err != nil {
		return nil, err
	}

	return resp.Results, nil
}

func (c *boincClient) send(req *boincRequest) (*boincReply, error) {
	reqData, err := xml.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	reqData = append(reqData, 3)

	if logger.Level.Enabled(slog.LevelDebug) {
		c.Debugf("sending request: %s", string(reqData))
	}

	const (
		respStart = "<boinc_gui_rpc_reply>"
		respEnd   = "</boinc_gui_rpc_reply>"
	)

	var b bytes.Buffer

	if err := c.conn.Command(string(reqData), func(bs []byte) (bool, error) {
		s := strings.TrimSpace(string(bs))
		if s == "" {
			return true, nil
		}

		if b.Len() == 0 && s != respStart {
			return false, fmt.Errorf("unexpected response first line: %s", s)
		}

		b.WriteString(s)

		return s != respEnd, nil
	}); err != nil {
		return nil, fmt.Errorf("failed to send command: %v", err)
	}

	if logger.Level.Enabled(slog.LevelDebug) {
		c.Debugf("received response: %s", string(b.Bytes()))
	}

	respData := cleanReplyData(b.Bytes())

	var resp boincReply

	if err := xml.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal reply: %v", err)
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("received error from server: %s", *resp.Error)
	}
	if resp.BadRequest != nil {
		return nil, errors.New("received bad request response from server")
	}
	if resp.Unauthorized != nil {
		return nil, errors.New("received unauthorized response from server")
	}

	return &resp, nil
}

func cleanReplyData(resp []byte) []byte {
	tags := []string{"bad_request", "authorized", "unauthorized", "have_credentials", "cookie_required"}
	s := expandEmptyTags(string(resp), tags)
	return []byte(strings.ReplaceAll(s, `encoding="ISO-8859-1"`, `encoding="UTF-8"`))
}

func makeNonceMD5(nonce, pass string) string {
	hex := fmt.Sprintf("%x", md5.Sum([]byte(nonce+pass)))
	return hex
}

func expandEmptyTags(xmlString string, tags []string) string {
	for _, tag := range tags {
		emptyTag := fmt.Sprintf("<%s/>", tag)
		expandedTag := fmt.Sprintf("<%s></%s>", tag, tag)
		xmlString = strings.ReplaceAll(xmlString, emptyTag, expandedTag)
		xmlString = strings.ReplaceAll(xmlString, fmt.Sprintf("<%s />", tag), expandedTag)
	}
	return xmlString
}
