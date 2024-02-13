// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
)

type decoder func(r io.Reader, s *status) error

func decodeJSON(r io.Reader, s *status) error {
	return json.NewDecoder(r).Decode(s)
}

func decodeText(r io.Reader, s *status) error {
	parts := readParts(r)
	if len(parts) == 0 {
		return errors.New("invalid text format")
	}

	part, parts := parts[0], parts[1:]
	if err := readStatus(part, s); err != nil {
		return err
	}

	return readProcesses(parts, s)
}

func readParts(r io.Reader) [][]string {
	sc := bufio.NewScanner(r)

	var parts [][]string
	var lines []string
	for sc.Scan() {
		line := strings.Trim(sc.Text(), "\r\n ")
		// Split parts by star border
		if strings.HasPrefix(line, "***") {
			parts = append(parts, lines)
			lines = []string{}
			continue
		}
		// Skip empty lines
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}

	if len(lines) > 0 {
		parts = append(parts, lines)
	}
	return parts
}

func readStatus(data []string, s *status) error {
	for _, line := range data {
		key, val, err := parseLine(line)
		if err != nil {
			return err
		}

		switch key {
		case "active processes":
			s.Active = parseInt(val)
		case "max active processes":
			s.MaxActive = parseInt(val)
		case "idle processes":
			s.Idle = parseInt(val)
		case "accepted conn":
			s.Requests = parseInt(val)
		case "max children reached":
			s.Reached = parseInt(val)
		case "slow requests":
			s.Slow = parseInt(val)
		}
	}
	return nil
}

func readProcesses(procs [][]string, s *status) error {
	for _, part := range procs {
		var proc proc
		for _, line := range part {
			key, val, err := parseLine(line)
			if err != nil {
				return err
			}

			switch key {
			case "state":
				proc.State = val
			case "request duration":
				proc.Duration = requestDuration(parseInt(val))
			case "last request cpu":
				proc.CPU = parseFloat(val)
			case "last request memory":
				proc.Memory = parseInt(val)
			}
		}
		s.Processes = append(s.Processes, proc)
	}
	return nil
}

func parseLine(s string) (string, string, error) {
	kv := strings.SplitN(s, ":", 2)
	if len(kv) != 2 {
		return "", "", errors.New("invalid text format line")
	}
	return strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1]), nil
}

func parseInt(s string) int64 {
	val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return val
}

func parseFloat(s string) float64 {
	val, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return val
}
