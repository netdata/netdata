// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var birdReplyLinePattern = regexp.MustCompile(`^(\d{4})([ -])(.*)$`)

type birdClient struct {
	socketPath string
	timeout    time.Duration

	mu     sync.Mutex
	conn   net.Conn
	reader *bufio.Reader
}

func (c *birdClient) ProtocolsAll() ([]byte, error) {
	return c.exec("show protocols all")
}

func (c *birdClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnect()
}

func (c *birdClient) exec(cmd string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for attempt := 0; attempt < 2; attempt++ {
		if err := c.connect(); err != nil {
			return nil, err
		}

		data, err := c.execWithConn(strings.TrimSpace(cmd))
		if err == nil {
			return data, nil
		}
		if isBirdCommandError(err) {
			return nil, err
		}

		_ = c.disconnect()
		if attempt == 1 {
			return nil, err
		}
	}

	return nil, fmt.Errorf("unexpected retry loop exit in exec")
}

func (c *birdClient) connect() error {
	if c.conn != nil && c.reader != nil {
		return nil
	}

	conn, err := (&net.Dialer{Timeout: c.timeout}).Dial("unix", c.socketPath)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	if err := conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		_ = conn.Close()
		return err
	}

	if _, _, _, err := readBIRDReply(reader); err != nil {
		_ = conn.Close()
		return err
	}

	c.conn = conn
	c.reader = reader
	return nil
}

func (c *birdClient) disconnect() error {
	var err error
	if c.conn != nil {
		err = c.conn.Close()
	}
	c.conn = nil
	c.reader = nil
	return err
}

func (c *birdClient) execWithConn(cmd string) ([]byte, error) {
	if err := c.conn.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return nil, err
	}

	if _, err := c.conn.Write([]byte(cmd + "\n")); err != nil {
		return nil, err
	}

	data, code, message, err := readBIRDReply(c.reader)
	if err != nil {
		return nil, err
	}
	if code == 16 || code >= 8000 {
		return nil, birdReplyError(code, message)
	}

	return data, nil
}

func readBIRDReply(reader *bufio.Reader) ([]byte, int, string, error) {
	var out bytes.Buffer

	var lastCode int
	var lastMessage string

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return bytes.TrimRight(out.Bytes(), "\n"), lastCode, lastMessage, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "+") {
			out.WriteString(line)
			out.WriteByte('\n')
			continue
		}

		match := birdReplyLinePattern.FindStringSubmatch(line)
		if match == nil {
			return nil, lastCode, lastMessage, fmt.Errorf("unexpected BIRD reply line %q", line)
		}

		code, _ := strconv.Atoi(match[1])
		sep := match[2]
		text := match[3]

		lastCode = code
		lastMessage = strings.TrimSpace(text)

		if code != 0 {
			out.WriteString(line)
			out.WriteByte('\n')
		}

		if sep == " " {
			return bytes.TrimRight(out.Bytes(), "\n"), lastCode, lastMessage, nil
		}
	}
}

type birdCommandError struct {
	err error
}

func (e birdCommandError) Error() string { return e.err.Error() }
func (e birdCommandError) Unwrap() error { return e.err }

func isBirdCommandError(err error) bool {
	var commandErr birdCommandError
	return errors.As(err, &commandErr)
}

func birdReplyError(code int, message string) error {
	text := strings.TrimSpace(message)
	if text == "" {
		text = fmt.Sprintf("BIRD reply code %04d", code)
	}

	switch code {
	case 16, 8007:
		return birdCommandError{err: fmt.Errorf("%w: %s", fs.ErrPermission, text)}
	default:
		return birdCommandError{err: fmt.Errorf("BIRD reply code %04d: %s", code, text)}
	}
}
