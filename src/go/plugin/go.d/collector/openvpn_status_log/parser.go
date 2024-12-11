// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type clientInfo struct {
	commonName     string
	bytesReceived  int64
	bytesSent      int64
	connectedSince int64
}

func parse(path string) ([]clientInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	_ = sc.Scan()
	line := sc.Text()

	if line == "OpenVPN CLIENT LIST" {
		return parseV1(sc), nil
	}
	if strings.HasPrefix(line, "TITLE,OpenVPN") || strings.HasPrefix(line, "TITLE\tOpenVPN") {
		return parseV2V3(sc), nil
	}
	if line == "OpenVPN STATISTICS" {
		return parseStaticKey(sc), nil
	}
	return nil, fmt.Errorf("the status log file is invalid (%s)", path)
}

func parseV1(sc *bufio.Scanner) []clientInfo {
	// https://github.com/OpenVPN/openvpn/blob/d5315a5d7400a26f1113bbc44766d49dd0c3688f/src/openvpn/multi.c#L836
	var clients []clientInfo

	for sc.Scan() {
		if !strings.HasPrefix(sc.Text(), "Common Name") {
			continue
		}
		for sc.Scan() && !strings.HasPrefix(sc.Text(), "ROUTING TABLE") {
			parts := strings.Split(sc.Text(), ",")
			if len(parts) != 5 {
				continue
			}

			name := parts[0]
			bytesRx, _ := strconv.ParseInt(parts[2], 10, 64)
			bytesTx, _ := strconv.ParseInt(parts[3], 10, 64)
			connSince, _ := time.Parse("Mon Jan 2 15:04:05 2006", parts[4])

			clients = append(clients, clientInfo{
				commonName:     name,
				bytesReceived:  bytesRx,
				bytesSent:      bytesTx,
				connectedSince: connSince.Unix(),
			})
		}
		break
	}
	return clients
}

func parseV2V3(sc *bufio.Scanner) []clientInfo {
	// https://github.com/OpenVPN/openvpn/blob/d5315a5d7400a26f1113bbc44766d49dd0c3688f/src/openvpn/multi.c#L901
	var clients []clientInfo
	var sep string
	if strings.IndexByte(sc.Text(), '\t') != -1 {
		sep = "\t"
	} else {
		sep = ","
	}

	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "CLIENT_LIST") {
			continue
		}
		parts := strings.Split(line, sep)
		if len(parts) != 13 {
			continue
		}

		name := parts[1]
		bytesRx, _ := strconv.ParseInt(parts[5], 10, 64)
		bytesTx, _ := strconv.ParseInt(parts[6], 10, 64)
		connSince, _ := strconv.ParseInt(parts[8], 10, 64)

		clients = append(clients, clientInfo{
			commonName:     name,
			bytesReceived:  bytesRx,
			bytesSent:      bytesTx,
			connectedSince: connSince,
		})
	}
	return clients
}

func parseStaticKey(sc *bufio.Scanner) []clientInfo {
	// https://github.com/OpenVPN/openvpn/blob/d5315a5d7400a26f1113bbc44766d49dd0c3688f/src/openvpn/sig.c#L283
	var info clientInfo
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "TCP/UDP") {
			continue
		}
		i := strings.IndexByte(line, ',')
		if i == -1 || len(line) == i {
			continue
		}
		bytes, _ := strconv.ParseInt(line[i+1:], 10, 64)
		switch line[:i] {
		case "TCP/UDP read bytes":
			info.bytesReceived += bytes
		case "TCP/UDP write bytes":
			info.bytesSent += bytes
		}
	}
	return []clientInfo{info}
}
