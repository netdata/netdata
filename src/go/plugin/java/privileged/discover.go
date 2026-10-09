// SPDX-License-Identifier: GPL-3.0-or-later

package privileged

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

func startTime(stat string) (uint64, error) {
	end := strings.LastIndex(stat, ") ")
	if end < 0 {
		return 0, fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(stat[end+2:])
	if len(fields) <= 19 {
		return 0, fmt.Errorf("short process stat")
	}
	return strconv.ParseUint(fields[19], 10, 64)
}

var errMixedCredentials = errors.New("mixed process credentials are unsupported")

func credentials(status string) (uid, gid uint32, err error) {
	seen := 0
	mixed := false
	for _, line := range strings.Split(status, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || (f[0] != "Uid:" && f[0] != "Gid:") {
			continue
		}
		if len(f) != 5 {
			return 0, 0, fmt.Errorf("invalid credentials")
		}
		var ids [4]uint32
		for i, v := range f[1:] {
			id, e := strconv.ParseUint(v, 10, 32)
			if e != nil {
				return 0, 0, e
			}
			ids[i] = uint32(id)
		}
		for _, id := range ids {
			if id != ids[1] {
				mixed = true
			}
		}
		if f[0] == "Uid:" {
			uid = ids[1]
			seen |= 1
		} else {
			gid = ids[1]
			seen |= 2
		}
	}
	if seen != 3 {
		return 0, 0, fmt.Errorf("missing credentials")
	}
	if mixed {
		return uid, gid, errMixedCredentials
	}
	return uid, gid, nil
}

// Restrict display names to a short, unambiguous subset of the agent option grammar.
func displayName(s string) string {
	var b strings.Builder
	for _, c := range s {
		if b.Len() >= 128 {
			break
		}
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("._-", c) {
			b.WriteRune(c)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func application(argv []string) (name string, helper bool) {
	for _, key := range []string{"--spring.application.name=", "-Dspring.application.name=", "-Dservice.name=", "--service.name=", "-Dotel.service.name="} {
		for _, arg := range argv {
			if strings.HasPrefix(arg, key) && len(arg) > len(key) {
				name = displayName(strings.TrimPrefix(arg, key))
				break
			}
		}
		if name != "" {
			break
		}
	}
	fallback := "java"
	for i := 1; i < len(argv); i++ {
		arg := argv[i]
		if arg == "-jar" && i+1 < len(argv) {
			v := strings.Split(argv[i+1], "/")
			fallback = v[len(v)-1]
			break
		}
		switch arg {
		case "-cp", "-classpath", "--class-path", "-p", "--module-path", "--add-modules", "--module", "-m":
			i++
			continue
		}
		if arg == "" || strings.HasPrefix(arg, "-") {
			continue
		}
		fallback = arg
		helper = arg == "NetdataAttach"
		break
	}
	if name == "" {
		name = displayName(fallback)
	}
	return name, helper
}
