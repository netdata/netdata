// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

const maxJobNameLen = 64

var trapJobNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var (
	errJobNameEmpty   = errors.New("job name is empty")
	errJobNameTooLong = fmt.Errorf("job name exceeds %d characters", maxJobNameLen)
	errJobNameNoMatch = errors.New("job name must match ^[a-zA-Z0-9][a-zA-Z0-9_-]*$")
)

func validateJobName(name string) error {
	if name == "" {
		return errJobNameEmpty
	}
	if len(name) > maxJobNameLen {
		return errJobNameTooLong
	}
	if !trapJobNameRE.MatchString(name) {
		return errJobNameNoMatch
	}
	return nil
}

func validateEndpoints(endpoints []EndpointConfig) error {
	if len(endpoints) == 0 {
		return errors.New("at least one endpoint is required")
	}

	seen := make(map[string]struct{}, len(endpoints))
	for i, ep := range endpoints {
		proto := strings.ToLower(ep.Protocol)
		switch proto {
		case "udp":
		default:
			return fmt.Errorf("endpoint %d: unsupported protocol %q (only udp is supported in M2)", i, proto)
		}

		if ep.Address == "" {
			return fmt.Errorf("endpoint %d: address is required", i)
		}

		if ep.Port < 1 || ep.Port > 65535 {
			return fmt.Errorf("endpoint %d: port must be between 1 and 65535, got %d", i, ep.Port)
		}

		addrStr := net.JoinHostPort(ep.Address, strconv.Itoa(ep.Port))
		udpAddr, err := net.ResolveUDPAddr("udp", addrStr)
		if err != nil {
			return fmt.Errorf("endpoint %d: invalid address/port %q: %v", i, addrStr, err)
		}
		key := proto + "/" + udpAddr.String()
		if _, ok := seen[key]; ok {
			return fmt.Errorf("endpoint %d: duplicate endpoint %q", i, key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validateVersions(versions []string) ([]string, error) {
	if len(versions) == 0 {
		return nil, errors.New("at least one SNMP version is required")
	}

	seen := make(map[string]struct{}, len(versions))
	normalized := make([]string, 0, len(versions))
	for i, version := range versions {
		version = strings.ToLower(strings.TrimSpace(version))
		switch version {
		case "v1", "v2c":
		default:
			return nil, fmt.Errorf("version %d: unsupported SNMP version %q (only v1 and v2c are supported in M2)", i, version)
		}
		if _, ok := seen[version]; ok {
			return nil, fmt.Errorf("version %d: duplicate SNMP version %q", i, version)
		}
		seen[version] = struct{}{}
		normalized = append(normalized, version)
	}
	return normalized, nil
}
