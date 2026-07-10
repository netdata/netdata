// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func TestDockerJSONToResolutionKeepsInspectValuesLineOriented(t *testing.T) {
	body := []byte(`{"Name":"/ignored","Config":{"Env":["NOMAD_NAMESPACE=prod\ninjected","NOMAD_JOB_NAME=api","NOMAD_TASK_NAME=worker","NOMAD_SHORT_ALLOC_ID=abc123"],"Image":"","Labels":{}}}`)
	result, ok := dockerJSONToResolution(body)
	if !ok {
		t.Fatal("docker JSON did not parse")
	}
	if got, want := result.name, "prod-api-worker-abc123"; got != want {
		t.Fatalf("name = %q, want the first physical inspect line %q", got, want)
	}
}

func TestDockerRetryFallbackPreservesInspectLabels(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "snap"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)

	id := strings.Repeat("7", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = w.Write([]byte(`{"Name":"","Config":{"Env":[],"Image":"registry.example.invalid/partial:v1","Labels":{"netdata.cloud/team":"platform"}}}`))
	}))
	defer server.Close()

	r := newResolver([]string{"cgroup-name"}, invocationConfig{
		dockerHost: server.URL,
		logLevel:   ndlpEmerg,
	})
	result, handled := r.resolveDockerID(context.Background(), id, "docker-"+id+".scope")
	if !handled {
		t.Fatal("valid Docker id was not handled")
	}
	if got, want := result.name, id[:12]; got != want {
		t.Fatalf("fallback name = %q, want %q", got, want)
	}
	if result.exitCode != exitRetry {
		t.Fatalf("exit code = %d, want retry", result.exitCode)
	}
	if got, want := result.labels.String(), `image="registry.example.invalid/partial:v1",netdata.cloud/team="platform"`; got != want {
		t.Fatalf("fallback labels:\nwant %q\n got %q", want, got)
	}
}

func TestPodmanRetryFallbackPreservesInspectLabels(t *testing.T) {
	id := strings.Repeat("8", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		_, _ = w.Write([]byte(`{"Name":"","Config":{"Env":[],"Image":"registry.example.invalid/podman-partial:v1","Labels":{"netdata.cloud/team":"platform"}}}`))
	}))
	defer server.Close()

	r := newResolver([]string{"cgroup-name"}, invocationConfig{
		podmanHost: server.URL,
		logLevel:   ndlpEmerg,
	})
	result, handled := r.resolvePodmanID(context.Background(), id, "libpod-"+id+".scope")
	if !handled || result.exitCode != exitRetry || result.name != id[:12] {
		t.Fatalf("handled=%v exit=%d name=%q", handled, result.exitCode, result.name)
	}
	if got, want := result.labels.String(), `image="registry.example.invalid/podman-partial:v1",netdata.cloud/team="platform"`; got != want {
		t.Fatalf("fallback labels:\nwant %q\n got %q", want, got)
	}
}

func TestDockerCLISelectionWithShortID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell command fixture is not available on Windows")
	}
	tmp := t.TempDir()
	id := strings.Repeat("9", 12)
	command := filepath.Join(tmp, "docker")
	script := `#!/bin/sh
printf '%s\n' "$1" "$2" "$3" > "$DOCKER_CALLS"
printf '%s\n' 'IMAGE_NAME=registry.example.invalid/cli:v1' 'CONT_NAME=/cli-container' 'LABEL_netdata.cloud/team="platform"'
`
	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	calls := filepath.Join(tmp, "docker.calls")
	t.Setenv("DOCKER_CALLS", calls)

	cgroup := "docker-" + id + ".scope"
	var out bytes.Buffer
	if code := runWithConfig([]string{"cgroup-name", cgroup, cgroup}, &out, invocationConfig{logLevel: ndlpEmerg}); code != exitSuccess {
		t.Fatalf("exit code = %d, want success", code)
	}
	if got, want := out.String(), `cli-container image="registry.example.invalid/cli:v1",netdata.cloud/team="platform"`+"\n"; got != want {
		t.Fatalf("stdout:\nwant %q\n got %q", want, got)
	}

	data, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	wantFormat := `--format={{range .Config.Env}}{{println .}}{{end}}{{range $key, $value := .Config.Labels}}LABEL_{{$key}}={{printf "%q" $value}}{{println}}{{end}}IMAGE_NAME={{.Config.Image}}{{println}}CONT_NAME={{.Name}}`
	wantCalls := "inspect\n" + wantFormat + "\n" + id + "\n"
	if got := string(data); got != wantCalls {
		t.Fatalf("docker arguments:\nwant %q\n got %q", wantCalls, got)
	}
}

func TestDockerAPIDeadlineRetriesWithoutFallback(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "snap"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	id := strings.Repeat("d", 64)
	cgroup := "docker-" + id + ".scope"
	var out bytes.Buffer
	code := runWithConfig([]string{"cgroup-name", cgroup, cgroup}, &out, invocationConfig{
		dockerHost: server.URL,
		logLevel:   ndlpEmerg,
		timeout:    25 * time.Millisecond,
	})
	if code != exitRetry {
		t.Fatalf("exit code = %d, want retry", code)
	}
	if out.Len() != 0 {
		t.Fatalf("deadline emitted fallback output %q", out.String())
	}
}

func TestDockerLikeGetNameAPIOverUnixSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix sockets are not available on Windows")
	}
	id := strings.Repeat("a", 64)
	temporary, err := os.CreateTemp("/tmp", "cgroup-name-socket-")
	if err != nil {
		t.Fatal(err)
	}
	socket := temporary.Name() + ".sock"
	_ = temporary.Close()
	_ = os.Remove(temporary.Name())
	t.Cleanup(func() { _ = os.Remove(socket) })
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got, want := request.URL.Path, "/containers/"+id+"/json"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		_, _ = w.Write([]byte(`{"Name":"/socket-container","Config":{"Env":[],"Image":"socket:v1","Labels":{}}}`))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	r := newResolver([]string{"cgroup-name"}, invocationConfig{logLevel: ndlpEmerg})
	result := r.dockerLikeGetNameAPI(context.Background(), "docker", "DOCKER_HOST", "unix://"+socket, id)
	if got, want := result.name, "socket-container"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if got, want := result.labels.String(), `image="socket:v1"`; got != want {
		t.Fatalf("labels = %q, want %q", got, want)
	}
}

func TestECSAndContainerdDispatchUseDockerAPI(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "snap"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tmp)
	id := strings.Repeat("b", 64)

	requests := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests <- request.URL.Path
		_, _ = w.Write([]byte(`{"Name":"/runtime-container","Config":{"Env":[],"Image":"runtime:v1","Labels":{}}}`))
	}))
	defer server.Close()
	r := newResolver([]string{"cgroup-name"}, invocationConfig{
		dockerHost: server.URL,
		logLevel:   ndlpEmerg,
	})

	for name, cgroup := range map[string]string{
		"ecs":        "ecs-a_task_" + id,
		"containerd": "system.slice_containerd.service_cpuset_" + id,
	} {
		t.Run(name, func(t *testing.T) {
			result, handled := r.resolveNonKubernetes(context.Background(), cgroup)
			if !handled || result.name != "runtime-container" {
				t.Fatalf("handled=%v name=%q", handled, result.name)
			}
			if got, want := <-requests, "/containers/"+id+"/json"; got != want {
				t.Fatalf("API path = %q, want %q", got, want)
			}
		})
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
