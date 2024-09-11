// SPDX-License-Identifier: GPL-3.0-or-later

package entnum

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
)

// https://www.iana.org/assignments/enterprise-numbers.txt
//
//go:embed "enterprise-numbers.txt"
var enterpriseNumberTxt []byte

func Lookup(number string) string {
	return numbers[number]
}

var numbers = func() map[string]string {
	if len(enterpriseNumberTxt) == 0 {
		panic("snmp: enterprise-numbers.txt is empty")
	}

	mapping := make(map[string]string, 65000)

	vr := strings.NewReplacer("\"", "", "`", "", "\\", "")
	var id string

	sc := bufio.NewScanner(bytes.NewReader(enterpriseNumberTxt))

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}

		if _, err := strconv.Atoi(line); err == nil {
			if id == "" {
				id = line
				if _, ok := mapping[id]; ok {
					panic("snmp: duplicate entry number: " + line)
				}
			}
			continue
		}
		if id != "" {
			line = vr.Replace(line)
			if line == "---none---" || line == "Reserved" {
				fmt.Println(id, line)
				id = ""
				continue
			}
			mapping[id] = line
			id = ""
		}
	}

	if len(mapping) == 0 {
		panic("snmp: enterprise-numbers mapping is empty after reading enterprise-numbers.txt")
	}

	return mapping
}()
