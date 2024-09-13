// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (p *Pihole) validateConfig() error {
	if p.URL == "" {
		return errors.New("url not set")
	}
	return nil
}

func (p *Pihole) initHTTPClient() (*http.Client, error) {
	return web.NewHTTPClient(p.ClientConfig)
}

func (p *Pihole) getWebPassword() string {
	// do no read setupVarsPath is password is set in the configuration file
	if p.Password != "" {
		return p.Password
	}
	if !isLocalHost(p.URL) {
		p.Info("abort web password auto detection, host is not localhost")
		return ""
	}

	p.Infof("starting web password auto detection, reading : %s", p.SetupVarsPath)
	pass, err := getWebPassword(p.SetupVarsPath)
	if err != nil {
		p.Warningf("error during reading '%s' : %v", p.SetupVarsPath, err)
	}

	return pass
}

func getWebPassword(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	s := bufio.NewScanner(f)
	var password string

	for s.Scan() && password == "" {
		if strings.HasPrefix(s.Text(), "WEBPASSWORD") {
			parts := strings.Split(s.Text(), "=")
			if len(parts) != 2 {
				return "", fmt.Errorf("unparsable line : %s", s.Text())
			}
			password = parts[1]
		}
	}

	return password, nil
}

func isLocalHost(u string) bool {
	if strings.Contains(u, "127.0.0.1") {
		return true
	}
	if strings.Contains(u, "localhost") {
		return true
	}

	return false
}
