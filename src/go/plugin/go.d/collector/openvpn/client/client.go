// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

var (
	reLoadStats = regexp.MustCompile(`^SUCCESS: nclients=([0-9]+),bytesin=([0-9]+),bytesout=([0-9]+)`)
	reVersion   = regexp.MustCompile(`^OpenVPN Version: OpenVPN ([0-9]+)\.([0-9]+)\.([0-9]+) .+Management Version: ([0-9])`)
)

const maxLinesToRead = 500

// New creates new OpenVPN client.
func New(config socket.Config) *Client {
	return &Client{Client: socket.New(config)}
}

// Client represents OpenVPN client.
type Client struct {
	socket.Client
}

// Users Users.
func (c *Client) Users() (Users, error) {
	lines, err := c.get(commandStatus3, readUntilEND)
	if err != nil {
		return nil, err
	}
	return decodeUsers(lines)
}

// LoadStats LoadStats.
func (c *Client) LoadStats() (*LoadStats, error) {
	lines, err := c.get(commandLoadStats, readOneLine)
	if err != nil {
		return nil, err
	}
	return decodeLoadStats(lines)
}

// Version Version.
func (c *Client) Version() (*Version, error) {
	lines, err := c.get(commandVersion, readUntilEND)
	if err != nil {
		return nil, err
	}
	return decodeVersion(lines)
}

func (c *Client) get(command string, stopRead stopReadFunc) (output []string, err error) {
	var num int
	if err := c.Command(command, func(bytes []byte) (bool, error) {
		line := string(bytes)
		num++
		if num > maxLinesToRead {
			return false, fmt.Errorf("read line limit exceeded (%d)", maxLinesToRead)
		}

		// skip real-time messages
		if strings.HasPrefix(line, ">") {
			return true, nil
		}

		line = strings.Trim(line, "\r\n ")
		output = append(output, line)
		if stopRead != nil && stopRead(line) {
			return false, nil
		}
		return true, nil
	}); err != nil {
		return nil, err
	}
	return output, err
}

type stopReadFunc func(string) bool

func readOneLine(_ string) bool { return true }

func readUntilEND(s string) bool { return strings.HasSuffix(s, "END") }

func decodeLoadStats(src []string) (*LoadStats, error) {
	m := reLoadStats.FindStringSubmatch(strings.Join(src, " "))
	if len(m) == 0 {
		return nil, fmt.Errorf("parse failed : %v", src)
	}
	return &LoadStats{
		NumOfClients: mustParseInt(m[1]),
		BytesIn:      mustParseInt(m[2]),
		BytesOut:     mustParseInt(m[3]),
	}, nil
}

func decodeVersion(src []string) (*Version, error) {
	m := reVersion.FindStringSubmatch(strings.Join(src, " "))
	if len(m) == 0 {
		return nil, fmt.Errorf("parse failed : %v", src)
	}
	return &Version{
		Major:      mustParseInt(m[1]),
		Minor:      mustParseInt(m[2]),
		Patch:      mustParseInt(m[3]),
		Management: mustParseInt(m[4]),
	}, nil
}

// works only for `status 3\n`
func decodeUsers(src []string) (Users, error) {
	var users Users

	// [CLIENT_LIST common_name 178.66.34.194:54200 10.9.0.5 9319 8978 Thu May 9 05:01:44 2019 1557345704 username]
	for _, v := range src {
		if !strings.HasPrefix(v, "CLIENT_LIST") {
			continue
		}
		parts := strings.Fields(v)
		// Right after the connection there are no virtual ip, and both common name and username UNDEF
		// CLIENT_LIST	UNDEF	178.70.95.93:39324		1411	3474	Fri May 10 07:41:54 2019	1557441714	UNDEF
		if len(parts) != 13 {
			continue
		}
		u := User{
			CommonName:     parts[1],
			RealAddress:    parts[2],
			VirtualAddress: parts[3],
			BytesReceived:  mustParseInt(parts[4]),
			BytesSent:      mustParseInt(parts[5]),
			ConnectedSince: mustParseInt(parts[11]),
			Username:       parts[12],
		}
		users = append(users, u)
	}
	return users, nil
}

func mustParseInt(str string) int64 {
	v, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		panic(err)
	}
	return v
}
