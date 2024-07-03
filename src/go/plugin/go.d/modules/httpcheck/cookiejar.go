// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

// TODO: implement proper cookie auth support
// relevant forum topic: https://community.netdata.cloud/t/howto-http-endpoint-collector-with-cookie-and-user-pass/3981/5?u=ilyam8

// cookie file format: https://everything.curl.dev/http/cookies/fileformat
func loadCookieJar(path string) (http.CookieJar, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, err
	}

	sc := bufio.NewScanner(file)

	for sc.Scan() {
		line, httpOnly := strings.CutPrefix(strings.TrimSpace(sc.Text()), "#HttpOnly_")

		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) != 6 && len(parts) != 7 {
			return nil, fmt.Errorf("got %d fields in line '%s', want 6 or 7", len(parts), line)
		}

		for i, v := range parts {
			parts[i] = strings.TrimSpace(v)
		}

		cookie := &http.Cookie{
			Domain:   parts[0],
			Path:     parts[2],
			Name:     parts[5],
			HttpOnly: httpOnly,
		}
		cookie.Secure, err = strconv.ParseBool(parts[3])
		if err != nil {
			return nil, err
		}
		expires, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil {
			return nil, err
		}
		if expires > 0 {
			cookie.Expires = time.Unix(expires, 0)
		}
		if len(parts) == 7 {
			cookie.Value = parts[6]
		}

		scheme := "http"
		if cookie.Secure {
			scheme = "https"
		}
		cookieURL := &url.URL{
			Scheme: scheme,
			Host:   cookie.Domain,
		}

		cookies := jar.Cookies(cookieURL)
		cookies = append(cookies, cookie)
		jar.SetCookies(cookieURL, cookies)
	}

	return jar, nil
}
