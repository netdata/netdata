// SPDX-License-Identifier: GPL-3.0-or-later

package tor

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

// https://spec.torproject.org/control-spec/index.html
// https://github.com/torproject/stem/blob/master/stem/control.py

const (
	cmdAuthenticate = "AUTHENTICATE"
	cmdQuit         = "QUIT"
	cmdGetInfo      = "GETINFO"
)

type controlConn interface {
	connect() error
	disconnect()

	getInfo(...string) ([]byte, error)
}

func newControlConn(conf Config) controlConn {
	return &torControlClient{
		password: conf.Password,
		conn: socket.New(socket.Config{
			Address: conf.Address,
			Timeout: conf.Timeout.Duration(),
		})}
}

type torControlClient struct {
	password string
	conn     socket.Client
}

func (c *torControlClient) connect() error {
	if err := c.conn.Connect(); err != nil {
		return err
	}

	return c.authenticate()
}

func (c *torControlClient) authenticate() error {
	// https://spec.torproject.org/control-spec/commands.html#authenticate

	cmd := cmdAuthenticate
	if c.password != "" {
		cmd = fmt.Sprintf("%s \"%s\"", cmdAuthenticate, c.password)
	}

	var s string
	err := c.conn.Command(cmd+"\n", func(bs []byte) (bool, error) {
		s = string(bs)
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("authentication failed: %v", err)
	}
	if !strings.HasPrefix(s, "250") {
		return fmt.Errorf("authentication failed: %s", s)
	}
	return nil
}

func (c *torControlClient) disconnect() {
	// https://spec.torproject.org/control-spec/commands.html#quit

	_ = c.conn.Command(cmdQuit+"\n", func(bs []byte) (bool, error) { return false, nil })
	_ = c.conn.Disconnect()
}

func (c *torControlClient) getInfo(keywords ...string) ([]byte, error) {
	// https://spec.torproject.org/control-spec/commands.html#getinfo

	if len(keywords) == 0 {
		return nil, errors.New("no keywords specified")
	}
	cmd := fmt.Sprintf("%s %s", cmdGetInfo, strings.Join(keywords, " "))

	var buf bytes.Buffer

	if err := c.conn.Command(cmd+"\n", func(bs []byte) (bool, error) {
		s := string(bs)

		switch {
		case strings.HasPrefix(s, "250-"):
			buf.WriteString(strings.TrimPrefix(s, "250-"))
			buf.WriteByte('\n')
			return true, nil
		case strings.HasPrefix(s, "250 "):
			return false, nil
		default:
			return false, errors.New(s)
		}
	}); err != nil {
		return nil, fmt.Errorf("command '%s' failed: %v", cmd, err)
	}

	return buf.Bytes(), nil
}
