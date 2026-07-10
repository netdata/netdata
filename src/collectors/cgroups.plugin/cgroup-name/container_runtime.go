// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type dockerInspect struct {
	Name   string `json:"Name"`
	Config struct {
		Env    []string        `json:"Env"`
		Image  string          `json:"Image"`
		Labels json.RawMessage `json:"Labels"`
	} `json:"Config"`
}

func parseDockerLikeInspectOutput(output string) resolution {
	values := make(map[string]string)
	var labels labelSet
	for line := range strings.SplitSeq(output, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch name {
		case "NOMAD_NAMESPACE", "NOMAD_JOB_NAME", "NOMAD_TASK_NAME", "NOMAD_SHORT_ALLOC_ID", "CONT_NAME", "IMAGE_NAME":
			values[name] = value
		default:
			if strings.HasPrefix(name, "LABEL_netdata.cloud/") {
				labels.add(strings.TrimPrefix(name, "LABEL_"), inspectLabelValue(value))
			}
		}
	}
	return containerResolution(values, labels)
}

func dockerJSONToResolution(body []byte) (resolution, bool) {
	var doc dockerInspect
	if err := json.Unmarshal(body, &doc); err != nil {
		return resolution{}, false
	}

	values := make(map[string]string)
	for _, env := range doc.Config.Env {
		name, value, ok := strings.Cut(env, "=")
		if ok {
			values[name] = value
		}
	}
	values["CONT_NAME"] = doc.Name
	values["IMAGE_NAME"] = doc.Config.Image

	entries, err := orderedStringEntries(doc.Config.Labels)
	if err != nil {
		return resolution{}, false
	}
	var labels labelSet
	for _, entry := range entries {
		if strings.HasPrefix(entry.key, "netdata.cloud/") {
			labels.add(entry.key, entry.value)
		}
	}
	return containerResolution(values, labels), true
}

func containerResolution(values map[string]string, cloudLabels labelSet) resolution {
	var name string
	if values["NOMAD_NAMESPACE"] != "" && values["NOMAD_JOB_NAME"] != "" &&
		values["NOMAD_TASK_NAME"] != "" && values["NOMAD_SHORT_ALLOC_ID"] != "" {
		name = values["NOMAD_NAMESPACE"] + "-" + values["NOMAD_JOB_NAME"] + "-" +
			values["NOMAD_TASK_NAME"] + "-" + values["NOMAD_SHORT_ALLOC_ID"]
	} else {
		name = strings.TrimPrefix(values["CONT_NAME"], "/")
	}

	var labels labelSet
	if values["IMAGE_NAME"] != "" {
		labels.add("image", values["IMAGE_NAME"])
	}
	labels.items = append(labels.items, cloudLabels.items...)
	return resolution{name: name, labels: labels}
}

func (r *resolver) dockerLikeGetNameCommand(ctx context.Context, command, id string) resolution {
	format := `{{range .Config.Env}}{{println .}}{{end}}{{range $key, $value := .Config.Labels}}LABEL_{{$key}}={{printf "%q" $value}}{{println}}{{end}}IMAGE_NAME={{.Config.Image}}{{println}}CONT_NAME={{.Name}}`
	defer r.track(command+"-inspect", time.Now())
	cmd := exec.CommandContext(ctx, command, "inspect", "--format="+format, id)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return resolution{}
	}
	return parseDockerLikeInspectOutput(string(out))
}

func (r *resolver) dockerLikeGetNameAPI(ctx context.Context, runtimeName, hostVar, host, containerID string) resolution {
	path := "/containers/" + containerID + "/json"
	if host == "" {
		r.warning(fmt.Sprintf("No %s is set", hostVar))
		return resolution{}
	}

	address := host
	if match := reHostScheme.FindStringSubmatch(host); len(match) > 2 {
		address = match[2]
	}

	defer r.track(runtimeName+"-api", time.Now())
	var body []byte
	var err error
	if isSocket(address) {
		r.info(fmt.Sprintf("Running API command: curl --unix-socket \"%s\" http://localhost%s", address, path))
		body, err = httpUnixGet(ctx, address, "http://localhost"+path)
	} else {
		r.info(fmt.Sprintf("Running API command: curl \"%s%s\"", address, path))
		body, err = httpGetWithContext(ctx, defaultHTTPURL(address+path), httpGetOptions{})
	}
	if err != nil || len(body) == 0 {
		return resolution{}
	}

	result, ok := dockerJSONToResolution(body)
	if !ok {
		return resolution{}
	}
	return result
}

func (r *resolver) snapHasDocker(ctx context.Context) bool {
	if !commandAvailable("snap") {
		return false
	}
	defer r.track("snap-list-docker", time.Now())
	return exec.CommandContext(ctx, "snap", "list", "docker").Run() == nil
}

func (r *resolver) resolveDockerID(ctx context.Context, id, cgroup string) (resolution, bool) {
	if id == "" || (len(id) != 64 && len(id) != 12) {
		r.error(fmt.Sprintf("a docker id cannot be extracted from docker cgroup '%s'.", cgroup))
		return resolution{}, false
	}

	var result resolution
	if r.snapHasDocker(ctx) {
		result = r.dockerLikeGetNameAPI(ctx, "docker", "DOCKER_HOST", r.config.dockerHost, id)
	} else if commandAvailable("docker") {
		result = r.dockerLikeGetNameCommand(ctx, "docker", id)
	} else {
		// Native JSON parsing replaces the shell's jq-gated Podman CLI fallback;
		// the Docker API branch is always the final resolver now.
		result = r.dockerLikeGetNameAPI(ctx, "docker", "DOCKER_HOST", r.config.dockerHost, id)
	}

	if result.name == "" {
		r.warning(fmt.Sprintf("cannot find the name of docker container '%s'", id))
		return resolution{name: prefixLen(id, 12), exitCode: exitRetry}, true
	}
	r.info(fmt.Sprintf("docker container '%s' is named '%s'", id, result.name))
	return result, true
}

func (r *resolver) resolvePodmanID(ctx context.Context, id, cgroup string) (resolution, bool) {
	if id == "" || len(id) != 64 {
		r.error(fmt.Sprintf("a podman id cannot be extracted from docker cgroup '%s'.", cgroup))
		return resolution{}, false
	}

	// The legacy Podman CLI leg ran only when jq was absent. Native JSON parsing
	// makes the API the complete and only Podman resolution path.
	result := r.dockerLikeGetNameAPI(ctx, "podman", "PODMAN_HOST", r.config.podmanHost, id)
	if result.name == "" {
		r.warning(fmt.Sprintf("cannot find the name of podman container '%s'", id))
		return resolution{name: prefixLen(id, 12), exitCode: exitRetry}, true
	}
	r.info(fmt.Sprintf("podman container '%s' is named '%s'", id, result.name))
	return result, true
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func prefixLen(value string, length int) string {
	if len(value) <= length {
		return value
	}
	return value[:length]
}
