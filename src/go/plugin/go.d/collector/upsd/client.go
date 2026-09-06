// SPDX-License-Identifier: GPL-3.0-or-later

package upsd

import (
	"encoding/csv"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

const (
	commandUsername = "USERNAME %s"
	commandPassword = "PASSWORD %s"
	commandListUPS  = "LIST UPS"
	commandListVar  = "LIST VAR %s"
	commandLogout   = "LOGOUT"
)

// https://github.com/networkupstools/nut/blob/81fca30b2998fa73085ce4654f075605ff0b9e01/docs/net-protocol.txt#L647
var errUpsdCommand = errors.New("upsd command error")

type upsdConn interface {
	connect() error
	disconnect() error
	authenticate(string, string) error
	upsUnits() ([]upsUnit, error)
}

type upsUnit struct {
	name string
	vars map[string]string
}

func newUpsdConn(conf Config) upsdConn {
	return &upsdClient{conn: socket.New(socket.Config{
		Timeout: conf.Timeout.Duration(),
		Address: conf.Address,
	})}
}

type upsdClient struct {
	conn socket.Client
}

func (c *upsdClient) connect() error {
	return c.conn.Connect()
}

func (c *upsdClient) disconnect() error {
	_, _ = c.sendCommand(commandLogout)
	return c.conn.Disconnect()
}

func (c *upsdClient) authenticate(username, password string) error {
	cmd := fmt.Sprintf(commandUsername, username)
	resp, err := c.sendCommand(cmd)
	if err != nil {
		return err
	}
	if resp[0] != "OK" {
		return errors.New("authentication failed: invalid username")
	}

	cmd = fmt.Sprintf(commandPassword, password)
	resp, err = c.sendCommand(cmd)
	if err != nil {
		return err
	}
	if resp[0] != "OK" {
		return errors.New("authentication failed: invalid password")
	}

	return nil
}

func (c *upsdClient) upsUnits() ([]upsUnit, error) {
	resp, err := c.sendCommand(commandListUPS)
	if err != nil {
		return nil, err
	}

	var upsNames []string

	for _, v := range resp {
		if !strings.HasPrefix(v, "UPS ") {
			continue
		}
		parts := splitLine(v)
		if len(parts) < 2 {
			continue
		}
		name := parts[1]
		upsNames = append(upsNames, name)
	}

	var upsUnits []upsUnit

	for _, name := range upsNames {
		cmd := fmt.Sprintf(commandListVar, name)
		resp, err := c.sendCommand(cmd)
		if err != nil {
			return nil, err
		}

		ups := upsUnit{
			name: name,
			vars: make(map[string]string),
		}

		upsUnits = append(upsUnits, ups)

		for _, v := range resp {
			if !strings.HasPrefix(v, "VAR ") {
				continue
			}
			parts := splitLine(v)
			if len(parts) < 4 {
				continue
			}
			n, v := parts[2], parts[3]
			ups.vars[n] = v
		}
	}

	return upsUnits, nil
}

func (c *upsdClient) sendCommand(cmd string) ([]string, error) {
	var resp []string
	var errMsg string
	endLine := getEndLine(cmd)

	err := c.conn.Command(cmd+"\n", func(bytes []byte) (bool, error) {
		line := string(bytes)
		resp = append(resp, line)

		if after, ok := strings.CutPrefix(line, "ERR "); ok {
			errMsg = after
		}

		return line != endLine && errMsg == "", nil
	})
	if err != nil {
		return nil, err
	}
	if errMsg != "" {
		return nil, fmt.Errorf("%w: %s (cmd: '%s')", errUpsdCommand, errMsg, commandName(cmd))
	}
	// Not errUpsdCommand: an empty response means the peer closed the
	// connection, so the caller must drop it, not keep using it.
	if len(resp) == 0 {
		return nil, fmt.Errorf("no response to command '%s'", commandName(cmd))
	}

	return resp, nil
}

func getEndLine(cmd string) string {
	switch commandName(cmd) {
	case "USERNAME", "PASSWORD", "VER":
		return "OK"
	}
	return fmt.Sprintf("END %s", cmd)
}

// commandName returns the command verb without its arguments. USERNAME and
// PASSWORD carry credentials in their arguments, so only the verb may be put
// in an error or a log line.
func commandName(cmd string) string {
	name, _, _ := strings.Cut(cmd, " ")
	return name
}

func splitLine(s string) []string {
	r := csv.NewReader(strings.NewReader(s))
	r.Comma = ' '

	parts, _ := r.Read()

	return parts
}
