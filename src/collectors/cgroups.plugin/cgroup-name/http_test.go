// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	t.Run("body cap", func(t *testing.T) {
		_, err := httpGetWithContext(context.Background(), server.URL+"/large", httpGetOptions{maxBody: 4})
		if err == nil || !strings.Contains(err.Error(), "response exceeds 4 bytes") {
			t.Fatalf("body cap error = %v", err)
		}
	})

	t.Run("non-2xx is optional", func(t *testing.T) {
		body, err := httpGetWithContext(context.Background(), server.URL+"/failure", httpGetOptions{})
		if err != nil || !strings.Contains(string(body), "upstream failure") {
			t.Fatalf("non-failing status: body=%q err=%v", body, err)
		}
		body, err = httpGetWithContext(context.Background(), server.URL+"/failure", httpGetOptions{fail: true})
		if err == nil || !strings.Contains(err.Error(), "http status 503") || !strings.Contains(string(body), "upstream failure") {
			t.Fatalf("failing status: body=%q err=%v", body, err)
		}
	})
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
	if _, err := httpGetWithContext(context.Background(), server.URL, httpGetOptions{
		tlsConfig: secure.k8sTLSConfig(tlsModeKubelet),
		noProxy:   true,
		fail:      true,
	}); err != nil {
		t.Fatalf("kubelet mode must accept a self-signed certificate, got: %v", err)
	}
	if _, err := httpGetWithContext(context.Background(), server.URL, httpGetOptions{
		tlsConfig: secure.k8sTLSConfig(tlsModeAPIServer),
		noProxy:   true,
		fail:      true,
	}); err == nil {
		t.Fatal("API-server mode must reject a certificate that does not chain to the service-account CA")
	}

	insecure := newResolver([]string{"cgroup-name"}, invocationConfig{
		logLevel: ndlpInfo,
		kubernetes: kubernetesConfig{
			serviceAccountCAFile: k8sServiceAccountCAFile,
			tlsInsecure:          true,
		},
	})
	if _, err := httpGetWithContext(context.Background(), server.URL, httpGetOptions{
		tlsConfig: insecure.k8sTLSConfig(tlsModeAPIServer),
		noProxy:   true,
		fail:      true,
	}); err != nil {
		t.Fatalf("insecure API-server mode must accept any certificate, got: %v", err)
	}
}

func TestK8sTLSConfigIsReusedPerInvocation(t *testing.T) {
	r := newResolver([]string{"cgroup-name"}, invocationConfig{
		logLevel: ndlpEmerg,
		kubernetes: kubernetesConfig{
			tlsInsecure: true,
		},
	})
	if first, second := r.k8sTLSConfig(tlsModeAPIServer), r.k8sTLSConfig(tlsModeAPIServer); first != second {
		t.Fatal("API-server TLS roots were rebuilt within one invocation")
	}
	if first, second := r.k8sTLSConfig(tlsModeKubelet), r.k8sTLSConfig(tlsModeKubelet); first != second {
		t.Fatal("kubelet TLS configuration was rebuilt within one invocation")
	}
}

func TestKubeletPodsURLAppendsPods(t *testing.T) {
	if got := kubeletPodsURL(defaultKubeletURL); got != "https://localhost:10250/pods" {
		t.Fatalf("default kubelet url = %q", got)
	}
	if got := kubeletPodsURL("https://node-1:10250"); got != "https://node-1:10250/pods" {
		t.Fatalf("configured kubelet url = %q", got)
	}
}
