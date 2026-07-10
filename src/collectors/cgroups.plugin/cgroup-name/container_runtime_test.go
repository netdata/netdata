// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDockerInspectParser(t *testing.T) {
	result := parseDockerLikeInspectOutput(strings.Join([]string{
		"NOMAD_NAMESPACE=prod",
		"NOMAD_JOB_NAME=api",
		"NOMAD_TASK_NAME=worker",
		"NOMAD_SHORT_ALLOC_ID=abc123",
		"CONT_NAME=/ignored",
		"IMAGE_NAME=registry.example.invalid/app:1",
		"LABEL_netdata.cloud/service=payments",
		"LABEL_netdata.cloud/url=https://example.invalid/path?k=v",
		`LABEL_netdata.cloud/escaped="a,b\"c\nline"`,
	}, "\n"))

	if result.name != "prod-api-worker-abc123" {
		t.Fatalf("unexpected name %q", result.name)
	}
	wantLabels := `image="registry.example.invalid/app:1",netdata.cloud/service="payments",netdata.cloud/url="https://example.invalid/path?k=v",netdata.cloud/escaped="a,b\"c line"`
	if got := result.labels.String(); got != wantLabels {
		t.Fatalf("unexpected labels:\nwant %q\n got %q", wantLabels, got)
	}
}

func TestDockerJSONToResolutionPreservesLabelOrder(t *testing.T) {
	body := []byte(`{"Name":"/web","Config":{"Env":["A=B"],"Image":"img:v1","Labels":{"netdata.cloud/a":"1","netdata.cloud/b":"2"}}}`)
	result, ok := dockerJSONToResolution(body)
	if !ok {
		t.Fatal("docker JSON did not parse")
	}
	if result.name != "web" {
		t.Fatalf("name = %q", result.name)
	}
	if got, want := result.labels.String(), `image="img:v1",netdata.cloud/a="1",netdata.cloud/b="2"`; got != want {
		t.Fatalf("labels:\nwant %q\n got %q", want, got)
	}
}

func TestMirroredDockerAndPodmanAPIFixtures(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "snap"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	t.Setenv("NETDATA_LOG_LEVEL", "emerg")

	inspectJSON := func(name, image string) string {
		return `{"Name":"/` + name + `","Config":{"Env":["PATH=/usr/bin"],"Image":"` + image + `","Labels":{"com.docker.compose.service":"ignored","netdata.cloud/service":"payments","netdata.cloud/team":"platform"}}}`
	}

	t.Run("docker http api", func(t *testing.T) {
		id := strings.Repeat("4", 64)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/containers/"+id+"/json" {
				t.Fatalf("unexpected docker API path %s", request.URL.Path)
			}
			_, _ = w.Write([]byte(inspectJSON("api-docker", "registry.example.invalid/api:v1")))
		}))
		defer server.Close()
		t.Setenv("DOCKER_HOST", server.URL)
		t.Setenv("PODMAN_HOST", "")

		cgroup := "system.slice/docker-" + id + ".scope"
		var out bytes.Buffer
		code := run([]string{"cgroup-name", cgroup, cgroup}, &out)
		if code != exitSuccess {
			t.Fatalf("exit code: want %d got %d", exitSuccess, code)
		}
		got := strings.TrimSpace(out.String())
		for _, want := range []string{
			`api-docker `,
			`image="registry.example.invalid/api:v1"`,
			`netdata.cloud/service="payments"`,
			`netdata.cloud/team="platform"`,
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("output missing %q:\n%s", want, got)
			}
		}
		if strings.Contains(got, "com.docker.compose.service") {
			t.Fatalf("non-netdata label leaked into output:\n%s", got)
		}
	})

	t.Run("podman http api", func(t *testing.T) {
		id := strings.Repeat("5", 64)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if request.URL.Path != "/containers/"+id+"/json" {
				t.Fatalf("unexpected podman API path %s", request.URL.Path)
			}
			_, _ = w.Write([]byte(inspectJSON("api-podman", "registry.example.invalid/podman:v1")))
		}))
		defer server.Close()
		t.Setenv("DOCKER_HOST", "")
		t.Setenv("PODMAN_HOST", server.URL)

		cgroup := "user.slice/user-1000.slice/user-1000.service/user.slice/libpod-" + id + ".scope/container"
		var out bytes.Buffer
		code := run([]string{"cgroup-name", cgroup, cgroup}, &out)
		if code != exitSuccess {
			t.Fatalf("exit code: want %d got %d", exitSuccess, code)
		}
		got := strings.TrimSpace(out.String())
		for _, want := range []string{
			`api-podman `,
			`image="registry.example.invalid/podman:v1"`,
			`netdata.cloud/service="payments"`,
			`netdata.cloud/team="platform"`,
		} {
			if !strings.Contains(got, want) {
				t.Fatalf("output missing %q:\n%s", want, got)
			}
		}
	})
}
