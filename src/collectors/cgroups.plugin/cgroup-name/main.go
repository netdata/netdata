// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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

const k8sServiceAccountCAFile = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"

// package-level var so tests can point it at a fixture file
var k8sServiceAccountTokenFile = "/var/run/secrets/kubernetes.io/serviceaccount/token"

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
	reLibvirtQemu        = regexp.MustCompile(`machine_.*\.libvirt-qemu`)
	reLxcPayload         = regexp.MustCompile(`lxc\.payload\.(.*)`)
	reProxmoxConfName    = regexp.MustCompile(`\s*name\s*:\s*(.*)?$`)
	reProxmoxConfHost    = regexp.MustCompile(`\s*hostname\s*:\s*(.*)?$`)
)

type callRecord struct {
	label    string
	duration time.Duration
}

type resolver struct {
	args        []string
	stdout      io.Writer
	programName string
	cmdLine     string
	logLevel    int
	name        string
	labels      string
	exitCode    int

	// expiresAt is the whole-invocation deadline; budgetExpired() reports when
	// it has passed so an unresolved name triggers the parent's retry ladder.
	// Zero means unbounded. The matching context.Context is threaded through the
	// resolver methods as an argument rather than stored on the struct.
	expiresAt time.Time

	callsMu sync.Mutex
	calls   []callRecord
}

func main() {
	os.Exit(run(os.Args, os.Stdout))
}

func run(args []string, stdout io.Writer) int {
	setupEnvironment()
	r := newResolver(args, stdout)
	ctx, cancel := r.setupDeadline()
	defer cancel()

	// set the default docker/podman socket env vars when unset; the API paths
	// read them back via os.Getenv. The return value is intentionally unused.
	_ = defaultEnv("DOCKER_HOST", "unix:///var/run/docker.sock")
	_ = defaultEnv("PODMAN_HOST", "unix:///run/podman/podman.sock")

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
		r.k8sGetName(ctx, cgroupPath, cgroup)
	}

	if r.name == "" {
		r.dispatchNonK8s(ctx, cgroup)
		if r.name == "" {
			if r.budgetExpired() {
				// unresolved because discovery ran out of budget: exit with no
				// output so the parent's retry ladder runs, instead of locking
				// in the raw fallback name
				r.logCallBreakdown()
				return exitRetry
			}
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

// setupEnvironment mirrors the long-standing shell helper: it appends the
// standard system directories to the inherited PATH (rather than locking PATH
// down), so docker/kubectl/podman/jq/ps are found wherever the host installed
// them. The helper runs unprivileged (as the netdata user), like the shell
// helper it replaces.
func setupEnvironment() {
	path := os.Getenv("PATH") + ":/sbin:/usr/sbin:/usr/local/sbin"
	if sbindirPost != "" {
		path += ":" + sbindirPost
	}
	_ = os.Setenv("PATH", path)
	_ = os.Setenv("LC_ALL", "C")
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
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

var ndlpNames = map[int]string{
	ndlpEmerg: "emergency",
	ndlpAlert: "alert",
	ndlpErr:   "error",
	ndlpWarn:  "warning",
	ndlpInfo:  "info",
	ndlpDebug: "debug",
}

// logfmt to stderr, like go.d.plugin and cgroup-network: the daemon captures
// helper stderr, so a per-message journal connection (systemd-cat-native)
// is neither needed nor safe in a short-lived helper - a wedged journal
// could block name resolution
func (r *resolver) log(level int, message string) {
	if level > r.logLevel {
		return
	}
	fmt.Fprintf(os.Stderr, "time=%s comm=%s level=%s request=%q msg=%q\n",
		time.Now().Format("2006-01-02T15:04:05.000Z07:00"),
		r.programName, ndlpNames[level], r.cmdLine, message)
}

// NETDATA_CGROUP_NAME_TIMEOUT_MS carries the operator budget X from
// cgroups.plugin, which kills the helper at X plus a grace period; the
// helper must stop all discovery work by X on its own.
func (r *resolver) setupDeadline() (context.Context, context.CancelFunc) {
	ms, _ := strconv.ParseInt(os.Getenv("NETDATA_CGROUP_NAME_TIMEOUT_MS"), 10, 64)
	if ms <= 0 {
		// unbounded: a cancelable context with no deadline; the returned cancel
		// releases its resources when run() returns
		return context.WithCancel(context.Background())
	}
	r.expiresAt = time.Now().Add(time.Duration(ms) * time.Millisecond)
	return context.WithDeadline(context.Background(), r.expiresAt)
}

func (r *resolver) budgetExpired() bool {
	return !r.expiresAt.IsZero() && time.Now().After(r.expiresAt)
}

func (r *resolver) track(label string, started time.Time) {
	r.callsMu.Lock()
	r.calls = append(r.calls, callRecord{label: label, duration: time.Since(started)})
	r.callsMu.Unlock()
}

func (r *resolver) logCallBreakdown() {
	r.callsMu.Lock()
	var b strings.Builder
	for i := range r.calls {
		fmt.Fprintf(&b, " %s=%s", r.calls[i].label, r.calls[i].duration.Round(time.Millisecond))
	}
	r.callsMu.Unlock()

	breakdown := b.String()
	if breakdown == "" {
		breakdown = " (no external calls were attempted)"
	}
	r.error("name resolution budget expired; time spent per external call:" + breakdown)
}

func (r *resolver) info(message string)    { r.log(ndlpInfo, message) }
func (r *resolver) warning(message string) { r.log(ndlpWarn, message) }
func (r *resolver) error(message string)   { r.log(ndlpErr, message) }
func (r *resolver) fatal(message string)   { r.log(ndlpAlert, message) }

func (r *resolver) dispatchNonK8s(ctx context.Context, cgroup string) {
	switch {
	case reDockerDispatch.MatchString(cgroup):
		r.dockerValidateID(ctx, extractOrOriginal(reDockerExtract, cgroup, 1))
	case reECSDispatch.MatchString(cgroup):
		r.dockerValidateID(ctx, extractOrOriginal(reECSExtract, cgroup, 1))
	case reContainerdDispatch.MatchString(cgroup):
		r.dockerValidateID(ctx, extractOrOriginal(reContainerdExtract, cgroup, 1))
	case rePodmanDispatch.MatchString(cgroup):
		m := rePodmanExtract.FindStringSubmatch(cgroup)
		id := cgroup
		if len(m) > 2 {
			id = m[2]
		}
		r.podmanValidateID(ctx, id, cgroup)
	case reNspawn.MatchString(cgroup):
		r.name = reNspawn.ReplaceAllString(cgroup, "$1")
	case strings.Contains(cgroup, "machine.slice_machine") && strings.Contains(cgroup, "-lxc"):
		r.name = "lxc/" + lxcMachineName(cgroup)
	case strings.Contains(cgroup, "machine.slice_machine") && strings.Contains(cgroup, "-qemu"):
		r.name = "qemu_" + machineName(cgroup, "-qemu")
	case reLibvirtQemu.MatchString(cgroup):
		name := strings.TrimPrefix(cgroup, "machine_")
		name = strings.TrimSuffix(name, ".libvirt-qemu")
		name = strings.Replace(name, "-", "_", 1)
		r.name = "qemu_" + name
	case reProxmoxQemu.MatchString(cgroup) && isDir(hostPath("/etc/pve")):
		m := reProxmoxQemu.FindStringSubmatch(cgroup)
		filename := hostPath("/etc/pve/qemu-server/" + m[1] + ".conf")
		if fileReadable(filename) {
			r.name = "qemu_" + firstConfigValue(filename, reProxmoxConfName, "^name: ")
		} else {
			r.error(fmt.Sprintf("proxmox config file missing %s or netdata does not have read access.  Please ensure netdata is a member of www-data group.", filename))
		}
	case reProxmoxLXC.MatchString(cgroup) && isDir(hostPath("/etc/pve")):
		m := reProxmoxLXC.FindStringSubmatch(cgroup)
		filename := hostPath("/etc/pve/lxc/" + m[1] + ".conf")
		if fileReadable(filename) {
			r.name = firstConfigValue(filename, reProxmoxConfHost, "^hostname: ")
		} else {
			r.error(fmt.Sprintf("proxmox config file missing %s or netdata does not have read access.  Please ensure netdata is a member of www-data group.", filename))
		}
	case strings.Contains(cgroup, "lxc.payload"):
		r.name = reLxcPayload.ReplaceAllString(cgroup, "$1")
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
	name = reMachineIDSegment.ReplaceAllString(name, "")
	name = strings.ReplaceAll(name, "/x2d", "")
	name = strings.ReplaceAll(name, "_x2d", "")
	name = strings.ReplaceAll(name, ".scope", "")
	return name
}

var reMachineIDSegment = regexp.MustCompile(`[\/_]x2d[[:digit:]]*`)

// machineName mirrors the shell's sed pipeline: strip everything up to the
// marker, remove only the FIRST `[\/_]x2dNNN` segment (the libvirt machine id;
// the shell expression had no /g), then drop the remaining x2d markers keeping
// the bytes after them, then drop ".scope". A global digit strip would eat
// digits belonging to the machine name itself, collapsing names like web-01
// and web-02 both to "web".
func machineName(s, marker string) string {
	i := strings.LastIndex(s, marker)
	if i >= 0 {
		s = s[i+len(marker):]
	}
	if loc := reMachineIDSegment.FindStringIndex(s); loc != nil {
		s = s[:loc[0]] + s[loc[1]:]
	}
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
	for line := range strings.SplitSeq(string(data), "\n") {
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
	for line := range strings.SplitSeq(output, "\n") {
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
		r.labels = labelPair("image", vars["IMAGE_NAME"])
	}

	for line := range strings.SplitSeq(output, "\n") {
		if !strings.HasPrefix(line, "LABEL_netdata.cloud/") {
			continue
		}
		lname, lval, _ := strings.Cut(line, "=")
		lname = strings.TrimPrefix(lname, "LABEL_")
		label := labelPair(lname, inspectLabelValue(lval))
		if r.labels != "" {
			r.labels += "," + label
		} else {
			r.labels = label
		}
	}
}

func (r *resolver) dockerLikeGetNameCommand(ctx context.Context, command, id string) bool {
	format := `{{range .Config.Env}}{{println .}}{{end}}{{range $key, $value := .Config.Labels}}LABEL_{{$key}}={{printf "%q" $value}}{{println}}{{end}}IMAGE_NAME={{.Config.Image}}{{println}}CONT_NAME={{.Name}}`
	defer r.track(command+"-inspect", time.Now())
	cmd := exec.CommandContext(ctx, command, "inspect", "--format="+format, id)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		r.parseDockerLikeInspectOutput(string(out))
	}
	return true
}

func (r *resolver) dockerLikeGetNameAPI(ctx context.Context, hostVar, containerID string) bool {
	host := os.Getenv(hostVar)
	path := "/containers/" + containerID + "/json"

	if host == "" {
		r.warning(fmt.Sprintf("No %s is set", hostVar))
		return false
	}

	address := host
	if m := reHostScheme.FindStringSubmatch(host); len(m) > 2 {
		address = m[2]
	}

	var body []byte
	var err error
	defer r.track(strings.ToLower(strings.TrimSuffix(hostVar, "_HOST"))+"-api", time.Now())
	if isSocket(address) {
		r.info(fmt.Sprintf("Running API command: curl --unix-socket \"%s\" http://localhost%s", address, path))
		body, err = httpUnixGet(ctx, address, "http://localhost"+path)
	} else {
		r.info(fmt.Sprintf("Running API command: curl \"%s%s\"", address, path))
		body, err = httpGetWithContext(ctx, defaultHTTPURL(address+path), httpGetOptions{})
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

func httpUnixGet(ctx context.Context, socketPath, url string) ([]byte, error) {
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socketPath)
		},
	}
	client := &http.Client{Transport: tr}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, defaultBodyCap+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > defaultBodyCap {
		return nil, fmt.Errorf("response exceeds %d bytes", int64(defaultBodyCap))
	}
	return body, nil
}

// k8sTLSMode selects the TLS policy of an HTTPS call.
//
// The API server is verified against the mounted service-account CA, the
// in-cluster default of client-go and of netdata's own go.d collectors.
// The kubelet is never verified: stock kubelet serving certificates are
// self-signed, and even cluster-CA-signed ones carry only node-name/IP SANs,
// so verification of https://localhost:10250 fails on every cluster.
type k8sTLSMode int

const (
	tlsModeNone k8sTLSMode = iota
	tlsModeAPIServer
	tlsModeKubelet
)

// K8S_TLS_INSECURE disables verification for API-server calls - an escape
// hatch for clusters whose API endpoint does not verify against the mounted
// service-account CA (custom PKI, intercepting proxies). A security-relaxing
// flag must not treat "=false" as enabled, so parsing is falsey-aware, unlike
// the presence-only sibling variables.
func k8sTLSInsecure() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("K8S_TLS_INSECURE"))) {
	case "", "0", "false", "no":
		return false
	}
	return true
}

var k8sTLSInsecureWarned sync.Once

func (r *resolver) k8sTLSConfig(mode k8sTLSMode) *tls.Config {
	switch mode {
	case tlsModeKubelet:
		return &tls.Config{ // NOSONAR - kubelet serving certs are commonly self-signed; this preserves legacy helper compatibility.
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, // NOSONAR - kubelet serving certs are commonly self-signed; this preserves legacy helper compatibility.
		}
	case tlsModeAPIServer:
		if k8sTLSInsecure() {
			k8sTLSInsecureWarned.Do(func() {
				r.warning("K8S_TLS_INSECURE is set: TLS verification of Kubernetes API calls is disabled")
			})
			return &tls.Config{ // NOSONAR - explicit operator escape hatch for API endpoints that cannot verify against the mounted CA.
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, // NOSONAR - explicit operator escape hatch for API endpoints that cannot verify against the mounted CA.
			}
		}
		return k8sServiceAccountTLSConfig()
	}
	return nil
}

// response size caps: a wedged or misbehaving endpoint must not make the
// helper allocate without bound; pod lists on dense nodes are legitimately
// megabytes, everything else is small
const (
	defaultBodyCap = 16 << 20
	k8sPodsBodyCap = 64 << 20
)

// tlsHint returns an actionable operator hint when err is a TLS certificate
// verification failure, pointing at the K8S_TLS_INSECURE escape hatch.
func tlsHint(err error) string {
	var ce *tls.CertificateVerificationError
	var ua x509.UnknownAuthorityError
	var ci x509.CertificateInvalidError
	var he x509.HostnameError
	if errors.As(err, &ce) || errors.As(err, &ua) || errors.As(err, &ci) || errors.As(err, &he) {
		return " (certificate verification failed - set K8S_TLS_INSECURE=true to skip Kubernetes API-server verification)"
	}
	return ""
}

// httpGetOptions bundles the optional knobs of httpGetWithContext; the zero
// value is a plain proxied GET capped at defaultBodyCap with no TLS override.
type httpGetOptions struct {
	headers   map[string]string
	tlsConfig *tls.Config
	noProxy   bool
	fail      bool // return an error on non-2xx HTTP status
	timeout   time.Duration
	maxBody   int64
}

func httpGetWithContext(ctx context.Context, url string, opts httpGetOptions) ([]byte, error) {
	maxBody := opts.maxBody
	if maxBody <= 0 {
		maxBody = defaultBodyCap
	}
	tr := &http.Transport{}
	if opts.tlsConfig != nil {
		tr.TLSClientConfig = opts.tlsConfig
	}
	if opts.noProxy {
		tr.Proxy = nil
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}
	client := &http.Client{Transport: tr}
	if opts.timeout > 0 {
		client.Timeout = opts.timeout
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range opts.headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(body)) > maxBody {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBody)
	}
	if opts.fail && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
		return body, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return body, nil
}

func k8sServiceAccountTLSConfig() *tls.Config {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}

	if ca, err := os.ReadFile(k8sServiceAccountCAFile); err == nil {
		roots.AppendCertsFromPEM(ca)
	}

	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
	}
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
		lines = append(lines, "LABEL_"+kv.key+"="+strconv.Quote(kv.value))
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

func (r *resolver) snapHasDocker(ctx context.Context) bool {
	if _, err := exec.LookPath("snap"); err != nil {
		return false
	}
	defer r.track("snap-list-docker", time.Now())
	return exec.CommandContext(ctx, "snap", "list", "docker").Run() == nil
}

func (r *resolver) dockerGetName(ctx context.Context, id string) {
	if r.snapHasDocker(ctx) {
		r.dockerLikeGetNameAPI(ctx, "DOCKER_HOST", id)
	} else if commandAvailable("docker") {
		r.dockerLikeGetNameCommand(ctx, "docker", id)
	} else if !r.dockerLikeGetNameAPI(ctx, "DOCKER_HOST", id) {
		r.dockerLikeGetNameCommand(ctx, "podman", id)
	}

	if r.name == "" {
		r.warning(fmt.Sprintf("cannot find the name of docker container '%s'", id))
		r.exitCode = exitRetry
		r.name = prefixLen(id, 12)
	} else {
		r.info(fmt.Sprintf("docker container '%s' is named '%s'", id, r.name))
	}
}

func (r *resolver) dockerValidateID(ctx context.Context, id string) {
	if id != "" && (len(id) == 64 || len(id) == 12) {
		r.dockerGetName(ctx, id)
	} else {
		r.error(fmt.Sprintf("a docker id cannot be extracted from docker cgroup '%s'.", strings.ReplaceAll(argOrEmpty(r.args, 2), "/", "_")))
	}
}

func (r *resolver) podmanGetName(ctx context.Context, id string) {
	if !r.dockerLikeGetNameAPI(ctx, "PODMAN_HOST", id) {
		r.dockerLikeGetNameCommand(ctx, "podman", id)
	}

	if r.name == "" {
		r.warning(fmt.Sprintf("cannot find the name of podman container '%s'", id))
		r.exitCode = exitRetry
		r.name = prefixLen(id, 12)
	} else {
		r.info(fmt.Sprintf("podman container '%s' is named '%s'", id, r.name))
	}
}

func (r *resolver) podmanValidateID(ctx context.Context, id, cgroup string) {
	if id != "" && len(id) == 64 {
		r.podmanGetName(ctx, id)
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

// hostPath joins NETDATA_HOST_PREFIX with an absolute host path using string
// concatenation, like the shell helper did: with an empty prefix the result
// must stay absolute, while filepath.Join("", "etc/pve") would yield a
// relative path resolved against the helper's working directory.
func hostPath(path string) string {
	return os.Getenv("NETDATA_HOST_PREFIX") + path
}

func (r *resolver) k8sIsPauseContainer(cgroupPath string) bool {
	base := hostPath("/sys/fs/cgroup")
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
	// /proc is intentionally NOT prefixed with NETDATA_HOST_PREFIX, matching
	// the shell helper: containerized deployments run the plugin in the host
	// PID namespace, where the local /proc resolves host PIDs.
	comm, err := os.ReadFile(filepath.Join("/proc", procs[0], "comm"))
	if err != nil {
		return false
	}
	return strings.TrimRight(string(comm), "\n") == "pause"
}

func (r *resolver) k8sGCPGetClusterName(ctx context.Context) (string, bool) {
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
	defer r.track("gcp-metadata", time.Now())
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	ch := make(chan result, len(urls))
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			body, err := httpGetWithContext(ctx, url, httpGetOptions{headers: headers, noProxy: true, fail: true, timeout: 3 * time.Second})
			// the shell captured these with $(curl ...), which strips trailing
			// newlines, and tested the stripped value for emptiness
			value := strings.TrimSpace(string(body))
			if err != nil || value == "" {
				cancel()
			}
			ch <- result{idx: i, value: value, err: err}
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

func (r *resolver) k8sGetKubePodName(ctx context.Context, cgroupPath, id string) (string, int) {
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
		if after, ok := strings.CutPrefix(name, "kubepods_kubepods"); ok {
			name = "kubepods" + after
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

	tmpDir := os.Getenv("TMPDIR")
	if tmpDir == "" {
		tmpDir = "/tmp"
	}
	tmpCluster := filepath.Join(tmpDir, "netdata-cgroups-k8s-cluster-name")
	tmpSystemUID := filepath.Join(tmpDir, "netdata-cgroups-kubesystem-uid")
	tmpContainers := filepath.Join(tmpDir, "netdata-cgroups-containers")
	repairPrivateFileMode(tmpCluster)
	repairPrivateFileMode(tmpSystemUID)
	repairPrivateFileMode(tmpContainers)

	var kubeClusterName, kubeSystemUID, labels, containers string
	if cntrID != "" && isPrivateRegularFile(tmpCluster) && isPrivateRegularFile(tmpSystemUID) && isPrivateRegularFile(tmpContainers) {
		if matched, ok := grepFile(tmpContainers, cntrID, 1); ok {
			labels = matched
			kubeSystemUID = firstLineFile(tmpSystemUID)
			kubeClusterName = firstLineFile(tmpCluster)
		}
	}
	if labels == "" {
		kubeSystemUID = firstLineFile(tmpSystemUID)
		kubeClusterName = firstLineFile(tmpCluster)
		if kubeClusterName == "" {
			if value, ok := r.k8sGCPGetClusterName(ctx); ok {
				kubeClusterName = value
			} else {
				kubeClusterName = "unknown"
			}
		}
		var kubeSystemNS string
		var pods string
		var code int
		kubeSystemNS, pods, code = r.k8sFetchPods(ctx, fn, kubeSystemUID)
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
			_ = writePrivateFile(tmpCluster, []byte(kubeClusterName+"\n"))
		}
		if kubeSystemNS != "" && kubeSystemUID != "" {
			_ = writePrivateFile(tmpSystemUID, []byte(kubeSystemUID+"\n"))
		}
		_ = writePrivateFile(tmpContainers, []byte(containers+"\n"))
	}

	qosClass := "guaranteed"
	if m := reK8sQOSAny.FindStringSubmatch(cleanID); len(m) > 1 {
		qosClass = m[1]
	}

	if cntrID != "" {
		if labels == "" {
			var ok bool
			labels, ok = grepString(containers, cntrID, 1)
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
		labels += "," + labelPair("kind", "container")
		labels += "," + labelPair("qos_class", qosClass)
		if kubeSystemUID != "" && kubeSystemUID != "null" {
			labels += "," + labelPair("cluster_id", kubeSystemUID)
		}
		if kubeClusterName != "" && kubeClusterName != "unknown" {
			labels += "," + labelPair("cluster_name", kubeClusterName)
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
		labels += "," + labelPair("kind", "pod")
		labels += "," + labelPair("qos_class", qosClass)
		if kubeSystemUID != "" && kubeSystemUID != "null" {
			labels += "," + labelPair("cluster_id", kubeSystemUID)
		}
		if kubeClusterName != "" && kubeClusterName != "unknown" {
			labels += "," + labelPair("cluster_name", kubeClusterName)
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

// isPrivateRegularFile reports whether path is an existing regular file (not a
// symlink, directory, or device). The Kubernetes cache files live in a
// world-writable TMPDIR under predictable names, so before reading them we must
// reject a symlink planted there by another local user instead of following it.
func isPrivateRegularFile(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode().IsRegular()
}

// writePrivateFile writes data to path atomically with mode 0600. It creates a
// fresh, unguessable temp file in the destination directory (os.CreateTemp uses
// O_EXCL and mode 0600) and renames it over path. rename(2) replaces a symlink
// planted at the destination instead of following it, so a hostile symlink in a
// world-writable TMPDIR cannot redirect the write to another file.
func writePrivateFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".cgroup-name-cache-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func repairPrivateFileMode(path string) {
	st, err := os.Lstat(path)
	if err == nil && st.Mode().IsRegular() {
		_ = os.Chmod(path, 0o600)
	}
}

func firstLineFile(path string) string {
	if !isPrivateRegularFile(path) {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return firstLine(string(data))
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}
	return s
}

func grepFile(path, pattern string, max int) (string, bool) {
	if !isPrivateRegularFile(path) {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return grepString(string(data), pattern, max)
}

func grepString(s, pattern string, max int) (string, bool) {
	var matches []string
	for line := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
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

// kubeletPodsURL treats KUBELET_URL as a base URL and always appends /pods,
// matching the shell's `${KUBELET_URL:-https://localhost:10250}/pods`.
func kubeletPodsURL() string {
	base := os.Getenv("KUBELET_URL")
	if base == "" {
		base = "https://localhost:10250"
	}
	return base + "/pods"
}

func (r *resolver) k8sFetchPods(ctx context.Context, fn, kubeSystemUID string) (string, string, int) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" && os.Getenv("KUBERNETES_PORT_443_TCP_PORT") != "" {
		tokenBytes, _ := os.ReadFile(k8sServiceAccountTokenFile)
		// the shell read the token with $(<file), which strips trailing newlines;
		// net/http rejects header values containing a newline outright
		header := map[string]string{"Authorization": "Bearer " + strings.TrimSpace(string(tokenBytes))}
		host := os.Getenv("KUBERNETES_SERVICE_HOST") + ":" + os.Getenv("KUBERNETES_PORT_443_TCP_PORT")
		var kubeSystemNS string
		if kubeSystemUID == "" {
			url := "https://" + host + "/api/v1/namespaces/kube-system"
			started := time.Now()
			body, err := httpGetWithContext(ctx, url, httpGetOptions{headers: header, tlsConfig: r.k8sTLSConfig(tlsModeAPIServer), fail: true})
			r.track("k8s-api-namespace", started)
			if err != nil {
				r.warning(fmt.Sprintf("%s: error on curl '%s': %s.%s", fn, url, err.Error(), tlsHint(err)))
			} else {
				kubeSystemNS = string(body)
			}
		}
		var url string
		tlsMode := tlsModeAPIServer
		if os.Getenv("USE_KUBELET_FOR_PODS_METADATA") != "" {
			// KUBELET_URL is a base URL; the shell always appended /pods
			url = kubeletPodsURL()
			tlsMode = tlsModeKubelet
		} else {
			url = "https://" + host + "/api/v1/pods"
			if os.Getenv("MY_NODE_NAME") != "" {
				url += "?fieldSelector=spec.nodeName==" + os.Getenv("MY_NODE_NAME")
			}
		}
		started := time.Now()
		body, err := httpGetWithContext(ctx, url, httpGetOptions{headers: header, tlsConfig: r.k8sTLSConfig(tlsMode), fail: true, maxBody: k8sPodsBodyCap})
		r.track("k8s-pods", started)
		if err != nil {
			r.warning(fmt.Sprintf("%s: error on curl '%s': %s.%s", fn, url, err.Error(), tlsHint(err)))
			return kubeSystemNS, "", 1
		}
		return kubeSystemNS, string(body), 0
	}

	if r.kubeletProcessRunning(ctx) {
		if commandAvailable("kubectl") {
			// Quirk kept from the shell helper: when KUBE_CONFIG is unset, the
			// namespace query below runs with --kubeconfig="" (kubectl default
			// loading rules) and only the pods query gets the admin.conf
			// fallback, because the shell applied its default between the two
			// calls.
			kubeConfig := os.Getenv("KUBE_CONFIG")
			var kubeSystemNS string
			if kubeSystemUID == "" {
				out, err := r.kubectlOutput(ctx, kubeConfig, "get", "namespaces", "kube-system", "-o", "json")
				if err != nil {
					r.warning(fmt.Sprintf("%s: error on 'kubectl': %s.", fn, strings.TrimRight(string(out), "\n")))
				} else {
					kubeSystemNS = string(out)
				}
			}
			if _, ok := os.LookupEnv("KUBE_CONFIG"); !ok {
				kubeConfig = "/etc/kubernetes/admin.conf"
			}
			out, err := r.kubectlOutput(ctx, kubeConfig, "get", "pods", "--all-namespaces", "-o", "json")
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

func (r *resolver) kubeletProcessRunning(ctx context.Context) bool {
	defer r.track("ps-kubelet", time.Now())
	return exec.CommandContext(ctx, "ps", "-C", "kubelet").Run() == nil
}

func (r *resolver) kubectlOutput(ctx context.Context, kubeConfig string, args ...string) ([]byte, error) {
	defer r.track("kubectl", time.Now())
	full := append([]string{"--kubeconfig=" + kubeConfig}, args...)
	return exec.CommandContext(ctx, "kubectl", full...).CombinedOutput()
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
		var base strings.Builder
		base.WriteString(labelPair("namespace", ptrString(item.Metadata.Namespace)) + ",")
		base.WriteString(labelPair("pod_name", ptrString(item.Metadata.Name)) + ",")
		base.WriteString(labelPair("pod_uid", ptrString(item.Metadata.UID)) + ",")

		annotations, err := orderedStringEntries(item.Metadata.Annotations)
		if err != nil {
			return "", err
		}
		for _, ann := range annotations {
			if strings.HasPrefix(ann.key, "netdata.cloud/") {
				base.WriteString(labelPair(ann.key, ann.value) + ",")
			}
		}
		for _, owner := range item.Metadata.OwnerReferences {
			if owner.Controller != nil && *owner.Controller {
				base.WriteString(labelPair("controller_kind", ptrString(owner.Kind)) + ",")
				base.WriteString(labelPair("controller_name", ptrString(owner.Name)) + ",")
				break
			}
		}
		base.WriteString(labelPair("node_name", ptrString(item.Spec.NodeName)) + ",")
		for _, st := range item.Status.ContainerStatuses {
			line := base.String()
			line += labelPair("container_name", ptrString(st.Name)) + ","
			line += labelPair("container_id", strings.NewReplacer("docker://", "", "cri-o://", "", "containerd://", "").Replace(ptrString(st.ContainerID)))
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

func labelPair(name, value string) string {
	var b strings.Builder
	b.WriteString(name)
	b.WriteString(`="`)
	for _, r := range value {
		switch r {
		case '"', '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		case '\t', '\n', '\r':
			b.WriteRune(' ')
		default:
			if r < ' ' || r == 0x7f {
				b.WriteRune(' ')
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteRune('"')
	return b.String()
}

func inspectLabelValue(value string) string {
	if unquoted, err := strconv.Unquote(value); err == nil {
		return unquoted
	}
	return value
}

func splitLabels(labels string) []string {
	if labels == "" {
		return nil
	}

	var out []string
	start := 0
	inQuote := false
	escaped := false
	for i := 0; i < len(labels); i++ {
		switch labels[i] {
		case '\\':
			if inQuote {
				escaped = !escaped
				continue
			}
		case '"':
			if !escaped {
				inQuote = !inQuote
			}
		case ',':
			if !inQuote {
				if part := strings.TrimSpace(labels[start:i]); part != "" {
					out = append(out, part)
				}
				start = i + 1
			}
		}
		if labels[i] != '\\' || !inQuote {
			escaped = false
		}
	}
	if part := strings.TrimSpace(labels[start:]); part != "" {
		out = append(out, part)
	}
	return out
}

func labelNameValue(label string) (string, string, bool) {
	lname, lval, ok := strings.Cut(label, "=")
	if !ok || lname == "" || lval == "" {
		return "", "", false
	}

	value, err := strconv.Unquote(lval)
	if err == nil {
		return lname, value, true
	}

	if len(lval) >= 2 && lval[0] == '"' && lval[len(lval)-1] == '"' {
		return lname, lval[1 : len(lval)-1], true
	}

	return lname, lval, true
}

func getLblVal(labels, wantName string) string {
	for _, label := range splitLabels(labels) {
		lname, lval, ok := labelNameValue(label)
		if ok && lname == wantName {
			return lval
		}
	}
	return "null"
}

func addLblPrefix(labels, prefix string) string {
	if labels == "" {
		return ""
	}
	parts := splitLabels(labels)
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
	for _, label := range splitLabels(labels) {
		lname, _, _ := strings.Cut(label, "=")
		if lname != lblName {
			out = append(out, label)
		}
	}
	return strings.Join(out, ",")
}

func (r *resolver) k8sGetName(ctx context.Context, cgroupPath, id string) {
	kubepodName, code := r.k8sGetKubePodName(ctx, cgroupPath, id)
	switch code {
	case 0:
		kubepodName = "k8s_" + kubepodName
		name, labels := splitNameLabels(kubepodName)
		if name != labels {
			r.info(fmt.Sprintf("k8s_get_name: cgroup '%s' has chart name '%s', labels '%s'", id, name, labels))
			r.name = name
			r.labels = labels
		} else {
			r.info(fmt.Sprintf("k8s_get_name: cgroup '%s' has chart name '%s'", id, name))
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
	if before, after, ok := strings.Cut(s, " "); ok {
		return before, after
	}
	return s, s
}
