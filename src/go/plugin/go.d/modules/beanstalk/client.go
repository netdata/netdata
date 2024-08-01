// SPDX-License-Identifier: GPL-3.0-or-later

package beanstalk

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type beanstalkClient struct {
	conn net.Conn
}

func newBeanstalkConn(conf Config) beanstalkConn {
	conn, err := net.DialTimeout("tcp", conf.Address, conf.Timeout.Duration())
	if err != nil {
		return nil
	}
	return &beanstalkClient{conn: conn}
}

func (c *beanstalkClient) connect() error {
	if c.conn == nil {
		return fmt.Errorf("connection not initialized")
	}
	return nil
}

func (c *beanstalkClient) disconnect() {
	c.conn.Write([]byte("quit\r\n"))
}

func (c *beanstalkClient) queryStats() ([]byte, error) {
	_, err := c.conn.Write([]byte("stats\r\n"))
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(c.conn)
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	header = strings.TrimSpace(header)

	if !strings.HasPrefix(header, "OK ") {
		return nil, fmt.Errorf("unexpected response: %s", header)
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected header format: %s", header)
	}
	lengthStr := strings.TrimSpace(parts[1])
	dataLength, err := strconv.Atoi(lengthStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing length: %v", err)
	}

	data := make([]byte, dataLength)
	n, err := reader.Read(data)
	if err != nil {
		return nil, err
	}
	if n != dataLength {
		return nil, fmt.Errorf("expected %d bytes, read %d bytes", dataLength, n)
	}

	return data, nil
}

func (c *beanstalkClient) listTubes() ([]byte, error) {
	_, err := c.conn.Write([]byte("list-tubes\r\n"))
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(c.conn)
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	header = strings.TrimSpace(header)

	if !strings.HasPrefix(header, "OK ") {
		return nil, fmt.Errorf("unexpected response: %s", header)
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected header format: %s", header)
	}
	lengthStr := strings.TrimSpace(parts[1])
	dataLength, err := strconv.Atoi(lengthStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing length: %v", err)
	}

	data := make([]byte, dataLength)
	n, err := reader.Read(data)
	if err != nil {
		return nil, err
	}
	if n != dataLength {
		return nil, fmt.Errorf("expected %d bytes, read %d bytes", dataLength, n)
	}

	return data, nil
}

func (c *beanstalkClient) statsTube(tubeName string) ([]byte, error) {
	command := fmt.Sprintf("stats-tube %s\r\n", tubeName)
	_, err := c.conn.Write([]byte(command))
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(c.conn)
	header, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	header = strings.TrimSpace(header)

	if !strings.HasPrefix(header, "OK ") {
		return nil, fmt.Errorf("unexpected response: %s", header)
	}

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected header format: %s", header)
	}
	lengthStr := strings.TrimSpace(parts[1])
	dataLength, err := strconv.Atoi(lengthStr)
	if err != nil {
		return nil, fmt.Errorf("error parsing length: %v", err)
	}

	data := make([]byte, dataLength)
	n, err := reader.Read(data)
	if err != nil {
		return nil, err
	}
	if n != dataLength {
		return nil, fmt.Errorf("expected %d bytes, read %d bytes", dataLength, n)
	}

	return data, nil
}
