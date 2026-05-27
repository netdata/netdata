// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	ndlpEmerg   = 0
	ndlpAlert   = 1
	ndlpErr     = 3
	ndlpWarn    = 4
	ndlpInfo    = 6
	ndlpDebug   = 7
	exitSuccess = 0
	exitRetry   = 2
	exitDisable = 3
)

var (
	sbindirPost string

	reDockerDispatch     = regexp.MustCompile(`^.*docker[-_/\.][a-fA-F0-9]+[-_\.]?.*$`)
	reDockerExtract      = regexp.MustCompile(`^.*docker[-_/]([a-fA-F0-9]+)[-_\.]?.*$`)
	reECSDispatch        = regexp.MustCompile(`^.*ecs[-_/\.][a-fA-F0-9]+[-_\.]?.*$`)
	reECSExtract         = regexp.MustCompile(`^.*ecs[-_/].*[-_/]([a-fA-F0-9]+)[-_\.]?.*$`)
	reContainerdDispatch = regexp.MustCompile(`system.slice_containerd.service_cpuset_[a-fA-F0-9]+[-_\.]?.*$`)
	reContainerdExtract  = regexp.MustCompile(`^.*ystem.slice_containerd.service_cpuset_([a-fA-F0-9]+)[-_\.]?.*$`)
	rePodmanDispatch     = regexp.MustCompile(`^.*libpod-[a-fA-F0-9]+.*$`)
	rePodmanExtract      = regexp.MustCompile(`^.*libpod-(conmon-)?([a-fA-F0-9]+).*$`)
	reNspawn             = regexp.MustCompile(`.*machine\.slice[_/](.*)\.service`)
	reProxmoxQemu        = regexp.MustCompile(`qemu.slice_([0-9]+).scope`)
	reProxmoxLXC         = regexp.MustCompile(`lxc_([0-9]+)`)
	reK8sQOS             = regexp.MustCompile(`.+(besteffort|burstable|guaranteed)$`)
	reK8sCRIID           = regexp.MustCompile(`.+pod[a-f0-9_-]+_(docker|crio|cri-containerd)-([a-f0-9]+)$`)
	reK8sPlainID         = regexp.MustCompile(`.+pod[a-f0-9-]+_([a-f0-9]+)$`)
	reK8sPodUID          = regexp.MustCompile(`.+pod([a-f0-9_-]+)$`)
	reK8sQOSAny          = regexp.MustCompile(`.+(besteffort|burstable)`)
	reNullName           = regexp.MustCompile(`_null(_|$)`)
	reHostScheme         = regexp.MustCompile(`^([a-z]+)://(.*)`)
)

type resolver struct {
	args        []string
	stdout      io.Writer
	programName string
	cmdLine     string
	logLevel    int
	name        string
	labels      string
	exitCode    int
	dockerHost  string
	podmanHost  string
}

func main() {
	os.Exit(run(os.Args, os.Stdout))
}

func run(args []string, stdout io.Writer) int {
	setupEnvironment()
	r := newResolver(args, stdout)

	r.dockerHost = defaultEnv("DOCKER_HOST", "unix:///var/run/docker.sock")
	r.podmanHost = defaultEnv("PODMAN_HOST", "unix:///run/podman/podman.sock")

	var cgroupPath string
	if len(args) > 1 {
		cgroupPath = args[1]
	}
	var cgroup string
	if len(args) > 2 {
		cgroup = strings.ReplaceAll(args[2], "/", "_")
	}

	if cgroup == "" {
		r.fatal("called without a cgroup name. Nothing to do.")
		return 1
	}

	if r.name == "" && strings.Contains(cgroup, "kubepods") {
		r.k8sGetName(cgroupPath, cgroup)
	}

	if r.name == "" {
		r.dispatchNonK8s(cgroup)
		if r.name == "" {
			r.name = cgroup
		}
		if len(r.name) > 100 {
			r.name = r.name[:100]
		}
	}

	r.name = strings.ReplaceAll(r.name, " ", "_")
	r.info(fmt.Sprintf("cgroup '%s' is called '%s', labels '%s'", cgroup, r.name, r.labels))

	if r.labels != "" {
		fmt.Fprintf(r.stdout, "%s %s\n", r.name, r.labels)
	} else {
		fmt.Fprintf(r.stdout, "%s\n", r.name)
	}

	return r.exitCode
}

func setupEnvironment() {
	path := os.Getenv("PATH") + ":/sbin:/usr/sbin:/usr/local/sbin"
	if sbindirPost != "" {
		path += ":" + sbindirPost
	}
	_ = os.Setenv("PATH", path)
	_ = os.Setenv("LC_ALL", "C")
}

func newResolver(args []string, stdout io.Writer) *resolver {
	programName := "cgroup-name"
	if len(args) > 0 && args[0] != "" {
		programName = filepath.Base(args[0])
	}
	r := &resolver{
		args:        args,
		stdout:      stdout,
		programName: programName,
		cmdLine:     buildCmdLine(args),
		logLevel:    ndlpInfo,
		exitCode:    exitSuccess,
	}
	r.setLogMinPriority()
	return r
}

func buildCmdLine(args []string) string {
	if len(args) == 0 {
		return "'' "
	}
	var b strings.Builder
	b.WriteString("'")
	b.WriteString(args[0])
	b.WriteString("' ")
	for _, arg := range args[1:] {
		b.WriteString("'")
		b.WriteString(arg)
		b.WriteString("' ")
	}
	return b.String()
}

func defaultEnv(name, value string) string {
	if os.Getenv(name) == "" {
		_ = os.Setenv(name, value)
		return value
	}
	return os.Getenv(name)
}

func (r *resolver) setLogMinPriority() {
	switch strings.ToLower(os.Getenv("NETDATA_LOG_LEVEL")) {
	case "emerg", "emergency":
		r.logLevel = ndlpEmerg
	case "alert":
		r.logLevel = ndlpAlert
	case "err", "error":
		r.logLevel = ndlpErr
	case "warn", "warning":
		r.logLevel = ndlpWarn
	case "info":
		r.logLevel = ndlpInfo
	case "debug":
		r.logLevel = ndlpDebug
	}
}

func (r *resolver) log(level int, message string) {
	if level > r.logLevel {
		return
	}
	message = strings.ReplaceAll(message, "\n", "\\n")
	payload := fmt.Sprintf("INVOCATION_ID=%s\nSYSLOG_IDENTIFIER=%s\nPRIORITY=%d\nTHREAD_TAG=cgroup-name\nND_LOG_SOURCE=collector\nND_REQUEST=%s\nMESSAGE=%s\n\n",
		os.Getenv("NETDATA_INVOCATION_ID"), r.programName, level, r.cmdLine, message)

	cmd := exec.Command("systemd-cat-native", "--log-as-netdata")
	cmd.Stdin = strings.NewReader(payload)
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}

func (r *resolver) info(message string)    { r.log(ndlpInfo, message) }
func (r *resolver) warning(message string) { r.log(ndlpWarn, message) }
func (r *resolver) error(message string)   { r.log(ndlpErr, message) }
func (r *resolver) fatal(message string)   { r.log(ndlpAlert, message) }

func (r *resolver) dispatchNonK8s(cgroup string) {
	switch {
	case reDockerDispatch.MatchString(cgroup):
		r.dockerValidateID(extractOrOriginal(reDockerExtract, cgroup, 1))
	case reECSDispatch.MatchString(cgroup):
		r.dockerValidateID(extractOrOriginal(reECSExtract, cgroup, 1))
	case reContainerdDispatch.MatchString(cgroup):
		r.dockerValidateID(extractOrOriginal(reContainerdExtract, cgroup, 1))
	case rePodmanDispatch.MatchString(cgroup):
		m := rePodmanExtract.FindStringSubmatch(cgroup)
		id := cgroup
		if len(m) > 2 {
			id = m[2]
		}
		r.podmanValidateID(id, cgroup)
	case reNspawn.MatchString(cgroup):
		r.name = reNspawn.ReplaceAllString(cgroup, "$1")
	case strings.Contains(cgroup, "machine.slice_machine") && strings.Contains(cgroup, "-lxc"):
		r.name = "lxc/" + lxcMachineName(cgroup)
	case strings.Contains(cgroup, "machine.slice_machine") && strings.Contains(cgroup, "-qemu"):
		r.name = "qemu_" + machineName(cgroup, "-qemu")
	case regexp.MustCompile(`machine_.*\.libvirt-qemu`).MatchString(cgroup):
		name := strings.TrimPrefix(cgroup, "machine_")
		name = strings.TrimSuffix(name, ".libvirt-qemu")
		name = strings.Replace(name, "-", "_", 1)
		r.name = "qemu_" + name
	case reProxmoxQemu.MatchString(cgroup) && isDir(filepath.Join(os.Getenv("NETDATA_HOST_PREFIX"), "etc/pve")):
		m := reProxmoxQemu.FindStringSubmatch(cgroup)
		filename := filepath.Join(os.Getenv("NETDATA_HOST_PREFIX"), "etc/pve/qemu-server", m[1]+".conf")
		if fileReadable(filename) {
			r.name = "qemu_" + firstConfigValue(filename, regexp.MustCompile(`\s*name\s*:\s*(.*)?$`), "^name: ")
		} else {
			r.error(fmt.Sprintf("proxmox config file missing %s or netdata does not have read access.  Please ensure netdata is a member of www-data group.", filename))
		}
	case reProxmoxLXC.MatchString(cgroup) && isDir(filepath.Join(os.Getenv("NETDATA_HOST_PREFIX"), "etc/pve")):
		m := reProxmoxLXC.FindStringSubmatch(cgroup)
		filename := filepath.Join(os.Getenv("NETDATA_HOST_PREFIX"), "etc/pve/lxc", m[1]+".conf")
		if fileReadable(filename) {
			r.name = firstConfigValue(filename, regexp.MustCompile(`\s*hostname\s*:\s*(.*)?$`), "^hostname: ")
		} else {
			r.error(fmt.Sprintf("proxmox config file missing %s or netdata does not have read access.  Please ensure netdata is a member of www-data group.", filename))
		}
	case strings.Contains(cgroup, "lxc.payload"):
		r.name = regexp.MustCompile(`lxc\.payload\.(.*)`).ReplaceAllString(cgroup, "$1")
	}
}

func extractOrOriginal(re *regexp.Regexp, s string, idx int) string {
	m := re.FindStringSubmatch(s)
	if len(m) > idx {
		return m[idx]
	}
	return s
}

func lxcMachineName(s string) string {
	name := machineName(s, "-lxc")
	name = regexp.MustCompile(`[\/_]x2d[[:digit:]]*`).ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, "/x2d", "")
	name = strings.ReplaceAll(name, "_x2d", "")
	name = strings.ReplaceAll(name, ".scope", "")
	return name
}

func machineName(s, marker string) string {
	i := strings.LastIndex(s, marker)
	if i >= 0 {
		s = s[i+len(marker):]
	}
	s = regexp.MustCompile(`[\/_]x2d[[:digit:]]*`).ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "/x2d", "")
	s = strings.ReplaceAll(s, "_x2d", "")
	s = strings.ReplaceAll(s, ".scope", "")
	return s
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func fileReadable(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	_ = f.Close()
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func firstConfigValue(path string, sedRE *regexp.Regexp, grepPrefix string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, strings.TrimPrefix(grepPrefix, "^")) {
			continue
		}
		if m := sedRE.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
		return ""
	}
	return ""
}

func (r *resolver) parseDockerLikeInspectOutput(output string) {
	vars := map[string]string{}
	wanted := map[string]bool{
		"NOMAD_NAMESPACE":      true,
		"NOMAD_JOB_NAME":       true,
		"NOMAD_TASK_NAME":      true,
		"NOMAD_SHORT_ALLOC_ID": true,
		"CONT_NAME":            true,
		"IMAGE_NAME":           true,
	}
	for _, line := range strings.Split(output, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if ok && wanted[name] {
			vars[name] = value
		}
	}

	if vars["NOMAD_NAMESPACE"] != "" && vars["NOMAD_JOB_NAME"] != "" &&
		vars["NOMAD_TASK_NAME"] != "" && vars["NOMAD_SHORT_ALLOC_ID"] != "" {
		r.name = vars["NOMAD_NAMESPACE"] + "-" + vars["NOMAD_JOB_NAME"] + "-" + vars["NOMAD_TASK_NAME"] + "-" + vars["NOMAD_SHORT_ALLOC_ID"]
	} else {
		r.name = strings.TrimPrefix(vars["CONT_NAME"], "/")
	}

	if vars["IMAGE_NAME"] != "" {
		r.labels = `image="` + vars["IMAGE_NAME"] + `"`
	}

	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "LABEL_netdata.cloud/") {
			continue
		}
		lname, lval, _ := strings.Cut(line, "=")
		lname = strings.TrimPrefix(lname, "LABEL_")
		label := lname + `="` + lval + `"`
		if r.labels != "" {
			r.labels += "," + label
		} else {
			r.labels = label
		}
	}
}

func (r *resolver) dockerLikeGetNameCommand(command, id string) bool {
	format := "{{range .Config.Env}}{{println .}}{{end}}{{range $key, $value := .Config.Labels}}LABEL_{{$key}}={{$value}}{{println}}{{end}}IMAGE_NAME={{.Config.Image}}{{println}}CONT_NAME={{.Name}}"
	cmd := exec.Command(command, "inspect", "--format="+format, id)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		r.parseDockerLikeInspectOutput(string(out))
	}
	return true
}

func (r *resolver) dockerLikeGetNameAPI(hostVar, containerID string) bool {
	host := os.Getenv(hostVar)
	path := "/containers/" + containerID + "/json"

	if host == "" {
		r.warning(fmt.Sprintf("No %s is set", hostVar))
		return false
	}

	if _, err := exec.LookPath("jq"); err != nil {
		r.warning(fmt.Sprintf("Can't find jq command line tool. jq is required for netdata to retrieve container name using %s API, falling back to docker ps", host))
		return false
	}

	address := host
	if m := reHostScheme.FindStringSubmatch(host); len(m) > 2 {
		address = m[2]
	}

	var body []byte
	var err error
	if isSocket(address) {
		r.info(fmt.Sprintf("Running API command: curl --unix-socket \"%s\" http://localhost%s", address, path))
		body, err = httpUnixGet(address, "http://localhost"+path)
	} else {
		r.info(fmt.Sprintf("Running API command: curl \"%s%s\"", address, path))
		body, err = httpGet(defaultHTTPURL(address+path), nil, false, false, false, 0)
	}
	if err != nil || len(body) == 0 {
		return true
	}

	if output, ok := dockerJSONToInspectOutput(body); ok && output != "" {
		r.parseDockerLikeInspectOutput(output)
	}
	return true
}

func isSocket(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode()&os.ModeSocket != 0
}

func defaultHTTPURL(url string) string {
	if strings.Contains(url, "://") {
		return url
	}
	return "http://" + url
}

func httpUnixGet(socketPath, url string) ([]byte, error) {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: tr}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func httpGet(url string, headers map[string]string, insecure, noProxy, fail bool, timeout time.Duration) ([]byte, error) {
	return httpGetWithContext(context.Background(), url, headers, insecure, noProxy, fail, timeout)
}

func httpGetWithContext(ctx context.Context, url string, headers map[string]string, insecure, noProxy, fail bool, timeout time.Duration) ([]byte, error) {
	tr := &http.Transport{}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // Mirrors curl -k in the legacy resolver.
	}
	if noProxy {
		tr.Proxy = nil
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}
	client := &http.Client{Transport: tr}
	if timeout > 0 {
		client.Timeout = timeout
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return nil, readErr
	}
	if fail && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return body, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return body, nil
}

type dockerInspect struct {
	Name   string `json:"Name"`
	Config struct {
		Env    []string        `json:"Env"`
		Image  string          `json:"Image"`
		Labels json.RawMessage `json:"Labels"`
	} `json:"Config"`
}

func dockerJSONToInspectOutput(body []byte) (string, bool) {
	var doc dockerInspect
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", false
	}
	var lines []string
	lines = append(lines, doc.Config.Env...)
	lines = append(lines, "CONT_NAME="+doc.Name)
	lines = append(lines, "IMAGE_NAME="+doc.Config.Image)
	labels, err := orderedStringEntries(doc.Config.Labels)
	if err != nil {
		return "", false
	}
	for _, kv := range labels {
		lines = append(lines, "LABEL_"+kv.key+"="+kv.value)
	}
	return strings.Join(lines, "\n"), true
}

type kv struct {
	key   string
	value string
}

func orderedStringEntries(raw json.RawMessage) ([]kv, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("not an object")
	}
	var out []kv
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("non-string key")
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		out = append(out, kv{key: key, value: rawJSONScalar(value)})
	}
	_, err = dec.Token()
	return out, err
}

func rawJSONScalar(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "null"
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}

func (r *resolver) dockerGetName(id string) {
	if _, err := exec.LookPath("snap"); err == nil && exec.Command("snap", "list", "docker").Run() == nil {
		r.dockerLikeGetNameAPI("DOCKER_HOST", id)
	} else if _, err := exec.LookPath("docker"); err == nil {
		r.dockerLikeGetNameCommand("docker", id)
	} else if !r.dockerLikeGetNameAPI("DOCKER_HOST", id) {
		r.dockerLikeGetNameCommand("podman", id)
	}

	if r.name == "" {
		r.warning(fmt.Sprintf("cannot find the name of docker container '%s'", id))
		r.exitCode = exitRetry
		r.name = prefixLen(id, 12)
	} else {
		r.info(fmt.Sprintf("docker container '%s' is named '%s'", id, r.name))
	}
}

func (r *resolver) dockerValidateID(id string) {
	if id != "" && (len(id) == 64 || len(id) == 12) {
		r.dockerGetName(id)
	} else {
		r.error(fmt.Sprintf("a docker id cannot be extracted from docker cgroup '%s'.", strings.ReplaceAll(argOrEmpty(r.args, 2), "/", "_")))
	}
}

func (r *resolver) podmanGetName(id string) {
	if !r.dockerLikeGetNameAPI("PODMAN_HOST", id) {
		r.dockerLikeGetNameCommand("podman", id)
	}

	if r.name == "" {
		r.warning(fmt.Sprintf("cannot find the name of podman container '%s'", id))
		r.exitCode = exitRetry
		r.name = prefixLen(id, 12)
	} else {
		r.info(fmt.Sprintf("podman container '%s' is named '%s'", id, r.name))
	}
}

func (r *resolver) podmanValidateID(id, cgroup string) {
	if id != "" && len(id) == 64 {
		r.podmanGetName(id)
	} else {
		r.error(fmt.Sprintf("a podman id cannot be extracted from docker cgroup '%s'.", cgroup))
	}
}

func prefixLen(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func argOrEmpty(args []string, idx int) string {
	if idx < len(args) {
		return args[idx]
	}
	return ""
}

func (r *resolver) k8sIsPauseContainer(cgroupPath string) bool {
	prefix := os.Getenv("NETDATA_HOST_PREFIX")
	base := filepath.Join(prefix, "sys/fs/cgroup")
	file := filepath.Join(base, cgroupPath, "cgroup.procs")
	if isDir(filepath.Join(base, "cpuacct")) {
		file = filepath.Join(base, "cpuacct", cgroupPath, "cgroup.procs")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return false
	}
	procs := strings.Fields(string(data))
	if len(procs) != 1 {
		return false
	}
	comm, err := os.ReadFile(filepath.Join("/proc", procs[0], "comm"))
	if err != nil {
		return false
	}
	return strings.TrimRight(string(comm), "\n") == "pause"
}

func (r *resolver) k8sGCPGetClusterName() (string, bool) {
	type result struct {
		idx   int
		value string
		err   error
	}
	urls := []string{
		"http://metadata/computeMetadata/v1/project/project-id",
		"http://metadata/computeMetadata/v1/instance/attributes/cluster-location",
		"http://metadata/computeMetadata/v1/instance/attributes/cluster-name",
	}
	headers := map[string]string{"Metadata-Flavor": "Google"}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch := make(chan result, len(urls))
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			body, err := httpGetWithContext(ctx, url, headers, false, true, true, 3*time.Second)
			if err != nil || len(body) == 0 {
				cancel()
			}
			ch <- result{idx: i, value: string(body), err: err}
		}(i, url)
	}
	wg.Wait()
	close(ch)

	values := make([]string, len(urls))
	for res := range ch {
		if res.err != nil || res.value == "" {
			return "", false
		}
		values[res.idx] = res.value
	}
	if values[0] == "" || values[1] == "" || values[2] == "" {
		return "", false
	}
	return "gke_" + values[0] + "_" + values[1] + "_" + values[2], true
}

func (r *resolver) k8sGetKubePodName(cgroupPath, id string) (string, int) {
	const fn = "k8s_get_kubepod_name"
	if !strings.Contains(id, "kubepods") {
		r.warning(fmt.Sprintf("%s: '%s' is not kubepod cgroup.", fn, id))
		return "", 1
	}

	cleanID := strings.ReplaceAll(id, ".slice", "")
	cleanID = strings.ReplaceAll(cleanID, ".scope", "")

	var name, podUID, cntrID string
	switch {
	case cleanID == "kubepods":
		name = cleanID
	case reK8sQOS.MatchString(cleanID):
		name = strings.ReplaceAll(cleanID, "-", "_")
		if strings.HasPrefix(name, "kubepods_kubepods") {
			name = "kubepods" + strings.TrimPrefix(name, "kubepods_kubepods")
		}
	case reK8sCRIID.MatchString(cleanID):
		cntrID = reK8sCRIID.FindStringSubmatch(cleanID)[2]
	case reK8sPlainID.MatchString(cleanID):
		cntrID = reK8sPlainID.FindStringSubmatch(cleanID)[1]
	case reK8sPodUID.MatchString(cleanID):
		podUID = strings.ReplaceAll(reK8sPodUID.FindStringSubmatch(cleanID)[1], "_", "-")
	}

	if name != "" {
		return name, 0
	}
	if podUID == "" && cntrID == "" {
		r.warning(fmt.Sprintf("%s: can't extract pod_uid or container_id from the cgroup '%s'.", fn, id))
		return "", 3
	}
	if podUID != "" {
		r.info(fmt.Sprintf("%s: cgroup '%s' is a pod(uid:%s)", fn, id, podUID))
	}
	if cntrID != "" {
		r.info(fmt.Sprintf("%s: cgroup '%s' is a container(id:%s)", fn, id, cntrID))
	}
	if cntrID != "" && r.k8sIsPauseContainer(cgroupPath) {
		return "", 3
	}
	if _, err := exec.LookPath("jq"); err != nil {
		r.warning(fmt.Sprintf("%s: 'jq' command not available.", fn))
		return "", 1
	}

	tmpDir := os.Getenv("TMPDIR")
	if tmpDir == "" {
		tmpDir = "/tmp"
	}
	tmpCluster := filepath.Join(tmpDir, "netdata-cgroups-k8s-cluster-name")
	tmpSystemUID := filepath.Join(tmpDir, "netdata-cgroups-kubesystem-uid")
	tmpContainers := filepath.Join(tmpDir, "netdata-cgroups-containers")

	var kubeClusterName, kubeSystemUID, labels, containers string
	if cntrID != "" && fileExists(tmpCluster) && fileExists(tmpSystemUID) && fileExists(tmpContainers) {
		if matched, ok := grepFile(tmpContainers, cntrID, 0); ok {
			labels = matched
			kubeSystemUID = firstLineFile(tmpSystemUID)
			kubeClusterName = firstLineFile(tmpCluster)
		}
	}
	if labels == "" {
		kubeSystemUID = firstLineFile(tmpSystemUID)
		kubeClusterName = firstLineFile(tmpCluster)
		if kubeClusterName == "" {
			if value, ok := r.k8sGCPGetClusterName(); ok {
				kubeClusterName = value
			} else {
				kubeClusterName = "unknown"
			}
		}
		var kubeSystemNS string
		var pods string
		var code int
		kubeSystemNS, pods, code = r.k8sFetchPods(fn, kubeSystemUID)
		if code != 0 {
			return "", code
		}
		if kubeSystemNS != "" {
			if uid, err := jsonMetadataUID(kubeSystemNS); err == nil {
				kubeSystemUID = uid
			} else {
				r.warning(fmt.Sprintf("%s: error on 'jq' parse kube_system_ns: %s.", fn, err.Error()))
			}
		}
		var err error
		containers, err = podsToContainerLines(pods)
		if err != nil {
			r.warning(fmt.Sprintf("%s: error on 'jq' parse pods: %s.", fn, err.Error()))
			return "", 1
		}
		if kubeClusterName != "" {
			_ = os.WriteFile(tmpCluster, []byte(kubeClusterName+"\n"), 0o644)
		}
		if kubeSystemNS != "" && kubeSystemUID != "" {
			_ = os.WriteFile(tmpSystemUID, []byte(kubeSystemUID+"\n"), 0o644)
		}
		_ = os.WriteFile(tmpContainers, []byte(containers+"\n"), 0o644)
	}

	qosClass := "guaranteed"
	if m := reK8sQOSAny.FindStringSubmatch(cleanID); len(m) > 1 {
		qosClass = m[1]
	}

	if cntrID != "" {
		if labels == "" {
			var ok bool
			labels, ok = grepString(containers, cntrID, 0)
			if !ok {
				return "", 2
			}
		}
		containerName := getLblVal(labels, "container_name")
		podName := getLblVal(labels, "pod_name")
		if strings.HasPrefix(podName, "virt-launcher-") {
			switch containerName {
			case "volumerootdisk", "guest-console-log":
				r.info(fmt.Sprintf("%s: skipping kubevirt helper container '%s' in pod '%s'", fn, containerName, podName))
				return "", 3
			}
		}
		labels += `,kind="container"`
		labels += `,qos_class="` + qosClass + `"`
		if kubeSystemUID != "" && kubeSystemUID != "null" {
			labels += `,cluster_id="` + kubeSystemUID + `"`
		}
		if kubeClusterName != "" && kubeClusterName != "unknown" {
			labels += `,cluster_name="` + kubeClusterName + `"`
		}
		name = "cntr_" + getLblVal(labels, "namespace") + "_" + getLblVal(labels, "pod_name") + "_" + getLblVal(labels, "container_name")
		labels = removeLbl(labels, "container_id")
		labels = removeLbl(labels, "pod_uid")
		labels = addLblPrefix(labels, "k8s_")
		name += " " + labels
	} else if podUID != "" {
		var ok bool
		labels, ok = grepString(containers, podUID, 1)
		if !ok {
			return "", 2
		}
		if i := strings.Index(labels, ",container_"); i >= 0 {
			labels = labels[:i]
		}
		labels += `,kind="pod"`
		labels += `,qos_class="` + qosClass + `"`
		if kubeSystemUID != "" && kubeSystemUID != "null" {
			labels += `,cluster_id="` + kubeSystemUID + `"`
		}
		if kubeClusterName != "" && kubeClusterName != "unknown" {
			labels += `,cluster_name="` + kubeClusterName + `"`
		}
		name = "pod_" + getLblVal(labels, "namespace") + "_" + getLblVal(labels, "pod_name")
		labels = removeLbl(labels, "pod_uid")
		labels = addLblPrefix(labels, "k8s_")
		name += " " + labels
	}

	if reNullName.MatchString(name) {
		r.warning(fmt.Sprintf("%s: invalid name: %s (cgroup '%s')", fn, name, id))
		if os.Getenv("USE_KUBELET_FOR_PODS_METADATA") != "" {
			return name, 2
		}
		return name, 1
	}
	if name != "" {
		return name, 0
	}
	return name, 1
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func firstLineFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return firstLine(string(data))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func grepFile(path, pattern string, max int) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return grepString(string(data), pattern, max)
}

func grepString(s, pattern string, max int) (string, bool) {
	var matches []string
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.Contains(line, pattern) {
			matches = append(matches, line)
			if max > 0 && len(matches) >= max {
				break
			}
		}
	}
	if len(matches) == 0 {
		return "", false
	}
	return strings.Join(matches, "\n"), true
}

func (r *resolver) k8sFetchPods(fn, kubeSystemUID string) (string, string, int) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_PORT_443_TCP_PORT") != "" {
		tokenBytes, _ := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
		header := map[string]string{"Authorization": "Bearer " + string(tokenBytes)}
		host := os.Getenv("KUBERNETES_SERVICE_HOST") + ":" + os.Getenv("KUBERNETES_PORT_443_TCP_PORT")
		var kubeSystemNS string
		if kubeSystemUID == "" {
			url := "https://" + host + "/api/v1/namespaces/kube-system"
			body, err := httpGet(url, header, true, false, true, 0)
			if err != nil {
				r.warning(fmt.Sprintf("%s: error on curl '%s': %s.", fn, url, err.Error()))
			} else {
				kubeSystemNS = string(body)
			}
		}
		url := os.Getenv("KUBELET_URL")
		if os.Getenv("USE_KUBELET_FOR_PODS_METADATA") != "" {
			if url == "" {
				url = "https://localhost:10250/pods"
			}
		} else {
			url = "https://" + host + "/api/v1/pods"
			if os.Getenv("MY_NODE_NAME") != "" {
				url += "?fieldSelector=spec.nodeName==" + os.Getenv("MY_NODE_NAME")
			}
		}
		body, err := httpGet(url, header, true, false, true, 0)
		if err != nil {
			r.warning(fmt.Sprintf("%s: error on curl '%s': %s.", fn, url, err.Error()))
			return kubeSystemNS, "", 1
		}
		return kubeSystemNS, string(body), 0
	}

	if exec.Command("ps", "-C", "kubelet").Run() == nil {
		if _, err := exec.LookPath("kubectl"); err == nil {
			kubeConfig := os.Getenv("KUBE_CONFIG")
			var kubeSystemNS string
			if kubeSystemUID == "" {
				out, err := kubectlOutput(kubeConfig, "get", "namespaces", "kube-system", "-o", "json")
				if err != nil {
					r.warning(fmt.Sprintf("%s: error on 'kubectl': %s.", fn, strings.TrimRight(string(out), "\n")))
				} else {
					kubeSystemNS = string(out)
				}
			}
			if _, ok := os.LookupEnv("KUBE_CONFIG"); !ok {
				kubeConfig = "/etc/kubernetes/admin.conf"
			}
			out, err := kubectlOutput(kubeConfig, "get", "pods", "--all-namespaces", "-o", "json")
			if err != nil {
				r.warning(fmt.Sprintf("%s: error on 'kubectl': %s.", fn, strings.TrimRight(string(out), "\n")))
				return kubeSystemNS, "", 1
			}
			return kubeSystemNS, string(out), 0
		}
	}

	r.warning(fmt.Sprintf("%s: not inside the k8s cluster and 'kubectl' command not available.", fn))
	return "", "", 1
}

func kubectlOutput(kubeConfig string, args ...string) ([]byte, error) {
	full := append([]string{"--kubeconfig=" + kubeConfig}, args...)
	cmd := exec.Command("kubectl", full...)
	return cmd.CombinedOutput()
}

func jsonMetadataUID(raw string) (string, error) {
	var doc struct {
		Metadata struct {
			UID *string `json:"uid"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", err
	}
	if doc.Metadata.UID == nil {
		return "null", nil
	}
	return *doc.Metadata.UID, nil
}

type podsDoc struct {
	Items []podItem `json:"items"`
}

type podItem struct {
	Metadata struct {
		Namespace       *string         `json:"namespace"`
		Name            *string         `json:"name"`
		UID             *string         `json:"uid"`
		Annotations     json.RawMessage `json:"annotations"`
		OwnerReferences []struct {
			Controller *bool   `json:"controller"`
			Kind       *string `json:"kind"`
			Name       *string `json:"name"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
	Spec struct {
		NodeName *string `json:"nodeName"`
	} `json:"spec"`
	Status struct {
		ContainerStatuses []struct {
			Name        *string `json:"name"`
			ContainerID *string `json:"containerID"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

func podsToContainerLines(raw string) (string, error) {
	var doc podsDoc
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", err
	}
	var lines []string
	for _, item := range doc.Items {
		base := `namespace="` + ptrString(item.Metadata.Namespace) + `",`
		base += `pod_name="` + ptrString(item.Metadata.Name) + `",`
		base += `pod_uid="` + ptrString(item.Metadata.UID) + `",`

		annotations, err := orderedStringEntries(item.Metadata.Annotations)
		if err != nil {
			return "", err
		}
		for _, ann := range annotations {
			if strings.HasPrefix(ann.key, "netdata.cloud/") {
				base += ann.key + `="` + ann.value + `",`
			}
		}
		for _, owner := range item.Metadata.OwnerReferences {
			if owner.Controller != nil && *owner.Controller {
				base += `controller_kind="` + ptrString(owner.Kind) + `",`
				base += `controller_name="` + ptrString(owner.Name) + `",`
				break
			}
		}
		base += `node_name="` + ptrString(item.Spec.NodeName) + `",`
		for _, st := range item.Status.ContainerStatuses {
			line := base
			line += `container_name="` + ptrString(st.Name) + `",`
			line += `container_id="` + strings.NewReplacer("docker://", "", "cri-o://", "", "containerd://", "").Replace(ptrString(st.ContainerID)) + `"`
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func ptrString(s *string) string {
	if s == nil {
		return "null"
	}
	return *s
}

func getLblVal(labels, wantName string) string {
	if labels == "" {
		return "null"
	}
	for _, label := range strings.Split(labels, ",") {
		lname, lval, ok := strings.Cut(label, "=")
		if ok && lname == wantName && lval != "" {
			if len(lval) >= 2 {
				return lval[1 : len(lval)-1]
			}
			return ""
		}
	}
	return "null"
}

func addLblPrefix(labels, prefix string) string {
	if labels == "" {
		return ""
	}
	parts := strings.Split(labels, ",")
	for i := range parts {
		parts[i] = prefix + parts[i]
	}
	return strings.Join(parts, ",")
}

func removeLbl(labels, lblName string) string {
	if labels == "" {
		return ""
	}
	var out []string
	for _, label := range strings.Split(labels, ",") {
		lname, _, _ := strings.Cut(label, "=")
		if lname != lblName {
			out = append(out, label)
		}
	}
	return strings.Join(out, ",")
}

func (r *resolver) k8sGetName(cgroupPath, id string) {
	kubepodName, code := r.k8sGetKubePodName(cgroupPath, id)
	switch code {
	case 0:
		kubepodName = "k8s_" + kubepodName
		name, labels := splitNameLabels(kubepodName)
		if name != labels {
			r.info(fmt.Sprintf("k8s_get_name: cgroup '%s' has chart name '%s', labels '%s", id, name, labels))
			r.name = name
			r.labels = labels
		} else {
			r.info(fmt.Sprintf("k8s_get_name: cgroup '%s' has chart name '%s'", id, r.name))
			r.name = name
		}
		r.exitCode = exitSuccess
	case 1:
		r.name = "k8s_" + id
		r.warning(fmt.Sprintf("k8s_get_name: cannot find the name of cgroup with id '%s'. Setting name to %s and enabling it.", id, r.name))
		r.exitCode = exitSuccess
	case 2:
		r.name = "k8s_" + id
		r.warning(fmt.Sprintf("k8s_get_name: cannot find the name of cgroup with id '%s'. Setting name to %s and asking for retry.", id, r.name))
		r.exitCode = exitRetry
	default:
		r.name = "k8s_" + id
		r.warning(fmt.Sprintf("k8s_get_name: cannot find the name of cgroup with id '%s'. Setting name to %s and disabling it.", id, r.name))
		r.exitCode = exitDisable
	}
}

func splitNameLabels(s string) (string, string) {
	if i := strings.IndexByte(s, ' '); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, s
}
