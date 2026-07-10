// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultBodyCap = 16 << 20
	k8sPodsBodyCap = 64 << 20
)

var reHostScheme = regexp.MustCompile(`^([a-z]+)://(.*)`)

type k8sTLSMode int

const (
	tlsModeAPIServer k8sTLSMode = iota
	tlsModeKubelet
)

type kubernetesTLSConfigCache struct {
	apiServerOnce sync.Once
	apiServer     *tls.Config
	kubeletOnce   sync.Once
	kubelet       *tls.Config
}

type httpGetOptions struct {
	headers   map[string]string
	tlsConfig *tls.Config
	noProxy   bool
	fail      bool
	timeout   time.Duration
	maxBody   int64
}

func isSocket(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

func defaultHTTPURL(url string) string {
	if strings.Contains(url, "://") {
		return url
	}
	return "http://" + url
}

func httpUnixGet(ctx context.Context, socketPath, url string) ([]byte, error) {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: transport}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, defaultBodyCap+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > defaultBodyCap {
		return nil, fmt.Errorf("response exceeds %d bytes", int64(defaultBodyCap))
	}
	return body, nil
}

func httpGetWithContext(ctx context.Context, url string, options httpGetOptions) ([]byte, error) {
	maxBody := options.maxBody
	if maxBody <= 0 {
		maxBody = defaultBodyCap
	}

	transport := &http.Transport{TLSClientConfig: options.tlsConfig}
	if options.noProxy {
		transport.Proxy = nil
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}
	client := &http.Client{Transport: transport, Timeout: options.timeout}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for name, value := range options.headers {
		request.Header.Set(name, value)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBody {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBody)
	}
	if options.fail && (response.StatusCode < 200 || response.StatusCode >= 300) {
		return body, fmt.Errorf("http status %d", response.StatusCode)
	}
	return body, nil
}

func (r *resolver) k8sTLSConfig(mode k8sTLSMode) *tls.Config {
	switch mode {
	case tlsModeKubelet:
		r.tlsConfigs.kubeletOnce.Do(func() {
			r.tlsConfigs.kubelet = &tls.Config{ // NOSONAR - kubelet serving certificates are commonly self-signed.
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, // NOSONAR - legacy-compatible kubelet behavior.
			}
		})
		return r.tlsConfigs.kubelet
	case tlsModeAPIServer:
		r.tlsConfigs.apiServerOnce.Do(func() {
			if r.config.kubernetes.tlsInsecure {
				r.warningf("K8S_TLS_INSECURE is set: TLS verification of Kubernetes API calls is disabled")
				r.tlsConfigs.apiServer = &tls.Config{ // NOSONAR - explicit operator escape hatch.
					MinVersion:         tls.VersionTLS12,
					InsecureSkipVerify: true, // NOSONAR - explicit operator escape hatch.
				}
				return
			}
			r.tlsConfigs.apiServer = k8sServiceAccountTLSConfig(r.config.kubernetes.serviceAccountCAFile)
		})
		return r.tlsConfigs.apiServer
	default:
		return nil
	}
}

func k8sServiceAccountTLSConfig(caFile string) *tls.Config {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if ca, err := os.ReadFile(caFile); err == nil {
		roots.AppendCertsFromPEM(ca)
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
}

func tlsHint(err error) string {
	_, isCertErr := errors.AsType[*tls.CertificateVerificationError](err)
	_, isUnknownAuth := errors.AsType[x509.UnknownAuthorityError](err)
	_, isInvalidCert := errors.AsType[x509.CertificateInvalidError](err)
	_, isHostname := errors.AsType[x509.HostnameError](err)
	if isCertErr || isUnknownAuth || isInvalidCert || isHostname {
		return " (certificate verification failed - set K8S_TLS_INSECURE=true to skip Kubernetes API-server verification)"
	}
	return ""
}
