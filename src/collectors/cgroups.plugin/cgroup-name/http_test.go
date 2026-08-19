// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPGetOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/large":
			_, _ = w.Write([]byte("12345"))
		case "/failure":
			http.Error(w, "upstream failure", http.StatusServiceUnavailable)
		default:
			_, _ = w.Write([]byte("ok"))
		}
	}))
	defer server.Close()

	tests := map[string]struct {
		path             string
		options          httpGetOptions
		wantBodyContains string
		wantErrContains  string
	}{
		"body cap": {
			path:            "/large",
			options:         httpGetOptions{maxBody: 4},
			wantErrContains: "response exceeds 4 bytes",
		},
		"non-2xx accepted": {
			path:             "/failure",
			wantBodyContains: "upstream failure",
		},
		"non-2xx rejected": {
			path:             "/failure",
			options:          httpGetOptions{fail: true},
			wantBodyContains: "upstream failure",
			wantErrContains:  "http status 503",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			body, err := httpGetWithContext(context.Background(), server.URL+test.path, test.options)
			if test.wantErrContains == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), test.wantErrContains)) {
				t.Fatalf("error = %v, want containing %q", err, test.wantErrContains)
			}
			if test.wantBodyContains != "" && !strings.Contains(string(body), test.wantBodyContains) {
				t.Fatalf("body = %q, want containing %q", body, test.wantBodyContains)
			}
		})
	}
}

func TestParseK8sTLSInsecure(t *testing.T) {
	tests := map[string]struct {
		value string
		want  bool
	}{
		"unset is secure":        {value: "", want: false},
		"zero is secure":         {value: "0", want: false},
		"false is secure":        {value: "false", want: false},
		"FALSE is secure":        {value: "FALSE", want: false},
		"no is secure":           {value: "no", want: false},
		"one enables":            {value: "1", want: true},
		"true enables":           {value: "true", want: true},
		"yes enables":            {value: "yes", want: true},
		"arbitrary text enables": {value: "on", want: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := parseK8sTLSInsecure(test.value); got != test.want {
				t.Fatalf("parseK8sTLSInsecure(%q) = %v, want %v", test.value, got, test.want)
			}
		})
	}
}

func TestK8sTLSModesAgainstSelfSignedServer(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	secure := newResolver([]string{"cgroup-name"}, invocationConfig{
		logLevel: ndlpInfo,
		kubernetes: kubernetesConfig{
			serviceAccountCAFile: k8sServiceAccountCAFile,
		},
	})
	insecure := newResolver([]string{"cgroup-name"}, invocationConfig{
		logLevel: ndlpInfo,
		kubernetes: kubernetesConfig{
			serviceAccountCAFile: k8sServiceAccountCAFile,
			tlsInsecure:          true,
		},
	})
	tests := map[string]struct {
		resolver *resolver
		mode     k8sTLSMode
		wantErr  bool
	}{
		"kubelet accepts self-signed certificate": {
			resolver: secure,
			mode:     tlsModeKubelet,
		},
		"secure API server rejects unknown CA": {
			resolver: secure,
			mode:     tlsModeAPIServer,
			wantErr:  true,
		},
		"insecure API server accepts self-signed certificate": {
			resolver: insecure,
			mode:     tlsModeAPIServer,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := httpGetWithContext(context.Background(), server.URL, httpGetOptions{
				tlsConfig: test.resolver.k8sTLSConfig(test.mode),
				noProxy:   true,
				fail:      true,
			})
			if test.wantErr && err == nil {
				t.Fatal("expected TLS error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected TLS error: %v", err)
			}
		})
	}
}

func TestK8sTLSConfigIsReusedPerInvocation(t *testing.T) {
	r := newResolver([]string{"cgroup-name"}, invocationConfig{
		logLevel: ndlpEmerg,
		kubernetes: kubernetesConfig{
			tlsInsecure: true,
		},
	})
	tests := map[string]struct {
		mode k8sTLSMode
	}{
		"API server": {mode: tlsModeAPIServer},
		"kubelet":    {mode: tlsModeKubelet},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if first, second := r.k8sTLSConfig(test.mode), r.k8sTLSConfig(test.mode); first != second {
				t.Fatal("TLS configuration was rebuilt within one invocation")
			}
		})
	}
}

func TestK8sServiceAccountTLSConfigUsesOnlyMountedCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	caFile := filepath.Join(t.TempDir(), "service-account-ca.crt")
	certificate := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	if err := os.WriteFile(caFile, certificate, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := k8sServiceAccountTLSConfig(caFile)
	if err != nil {
		t.Fatalf("load mounted cluster CA: %v", err)
	}
	if got := len(config.RootCAs.Subjects()); got != 1 {
		t.Fatalf("trusted CA subjects = %d, want only the mounted cluster CA", got)
	}
	if _, err := httpGetWithContext(context.Background(), server.URL, httpGetOptions{
		tlsConfig: config,
		noProxy:   true,
		fail:      true,
	}); err != nil {
		t.Fatalf("mounted cluster CA was not trusted: %v", err)
	}
}

func TestK8sServiceAccountTLSConfigRejectsInvalidCA(t *testing.T) {
	tmp := t.TempDir()
	invalidCA := filepath.Join(tmp, "invalid-ca.crt")
	if err := os.WriteFile(invalidCA, []byte("not a certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		caFile string
	}{
		"missing CA file": {
			caFile: filepath.Join(tmp, "missing-ca.crt"),
		},
		"invalid CA file": {
			caFile: invalidCA,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			config, err := k8sServiceAccountTLSConfig(test.caFile)
			if err == nil {
				t.Fatalf("CA load unexpectedly succeeded with %d trusted subjects", len(config.RootCAs.Subjects()))
			}
			if got := len(config.RootCAs.Subjects()); got != 0 {
				t.Fatalf("trusted CA subjects = %d after load failure, want 0", got)
			}
		})
	}
}

func TestKubeletPodsURLAppendsPods(t *testing.T) {
	tests := map[string]struct {
		base string
		want string
	}{
		"default URL": {
			base: defaultKubeletURL,
			want: "https://localhost:10250/pods",
		},
		"configured URL": {
			base: "https://node-1:10250",
			want: "https://node-1:10250/pods",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := kubeletPodsURL(test.base); got != test.want {
				t.Fatalf("kubelet URL = %q, want %q", got, test.want)
			}
		})
	}
}
