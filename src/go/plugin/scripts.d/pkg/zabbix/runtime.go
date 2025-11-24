// SPDX-License-Identifier: GPL-3.0-or-later

package zabbix

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/schedulers"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
	zpre "github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/zabbixpreproc"
)

// Runtime orchestrates Zabbix jobs using the shared scripts.d scheduler infrastructure.
type Runtime struct {
	proc    *zpre.Preprocessor
	handles map[string][]*schedulers.JobHandle
	mu      sync.RWMutex
}

// NewRuntime wires validated jobs into schedulers and starts them.
func NewRuntime(configs []JobConfig, proc *zpre.Preprocessor, log *logger.Logger, emitter runtime.ResultEmitter, vnodeLookup func(spec.JobSpec) runtime.VnodeInfo) (*Runtime, error) {
	if len(configs) == 0 {
		return nil, fmt.Errorf("zabbix runtime requires at least one job")
	}
	if proc == nil {
		return nil, fmt.Errorf("zabbix runtime requires a preprocessor instance")
	}

	normalized := make([]JobConfig, len(configs))
	for i := range configs {
		normalized[i] = configs[i]
		if strings.TrimSpace(normalized[i].Scheduler) == "" {
			normalized[i].Scheduler = "default"
		}
	}

	if emitter == nil {
		emitter = runtime.NewNoopEmitter()
	}
	collector := newJobCollector(normalized, log)
	grouped := groupByScheduler(normalized)
	handles := make(map[string][]*schedulers.JobHandle, len(grouped))

	detachAll := func(h map[string][]*schedulers.JobHandle) {
		for _, list := range h {
			for _, handle := range list {
				schedulers.DetachJob(handle)
			}
		}
	}

	for name, cfgs := range grouped {
		if _, ok := schedulers.Get(name); !ok {
			detachAll(handles)
			return nil, fmt.Errorf("scheduler '%s' not defined", name)
		}
		jobSpecs, err := buildJobSpecs(cfgs)
		if err != nil {
			detachAll(handles)
			return nil, err
		}
		for i := range jobSpecs {
			var vnode runtime.VnodeInfo
			if vnodeLookup != nil {
				vnode = runtime.CloneVnodeInfo(vnodeLookup(jobSpecs[i]))
			}
			reg := runtime.JobRegistration{
				Spec:    jobSpecs[i],
				Runner:  collector.Run,
				Emitter: emitter,
				Vnode:   vnode,
			}
			handle, err := schedulers.AttachJob(name, reg, log)
			if err != nil {
				detachAll(handles)
				return nil, err
			}
			handles[name] = append(handles[name], handle)
		}
	}

	return &Runtime{proc: proc, handles: handles}, nil
}

// Collect aggregates metrics from every scheduler.
func (r *Runtime) Collect() map[string]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	metrics := make(map[string]int64)
	for scheduler := range r.handles {
		data := schedulers.CollectMetrics(scheduler)
		for k, v := range data {
			metrics[k] += v
		}
	}
	if len(metrics) == 0 {
		return nil
	}
	return metrics
}

// Stop shuts down all managed schedulers and closes the emitter.
func (r *Runtime) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for scheduler, list := range r.handles {
		for _, handle := range list {
			schedulers.DetachJob(handle)
		}
		delete(r.handles, scheduler)
	}
}

func groupByScheduler(cfgs []JobConfig) map[string][]JobConfig {
	res := make(map[string][]JobConfig)
	for _, cfg := range cfgs {
		name := schedulerName(cfg.Scheduler)
		cfg.Scheduler = name
		res[name] = append(res[name], cfg)
	}
	return res
}

func schedulerName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "default"
	}
	return name
}

func buildJobSpecs(cfgs []JobConfig) ([]spec.JobSpec, error) {
	specs := make([]spec.JobSpec, len(cfgs))
	for i := range cfgs {
		cfg := cfgs[i]
		job := spec.JobConfig{
			Name:      cfg.Name,
			Plugin:    pluginName(cfg),
			Args:      append([]string{}, cfg.Collection.Args...),
			Timeout:   confopt.Duration(defaultTimeout(cfg)),
			Scheduler: cfg.Scheduler,
			Vnode:     cfg.Vnode,
		}
		job.CheckInterval = confopt.Duration(cfg.IntervalDuration())
		sp, err := job.ToSpec()
		if err != nil {
			return nil, err
		}
		specs[i] = sp
	}
	return specs, nil
}

func pluginName(cfg JobConfig) string {
	switch cfg.Collection.Type {
	case CollectionHTTP:
		return "zabbix-http"
	case CollectionSNMP:
		return "zabbix-snmp"
	case CollectionCommand, "":
		return strings.TrimSpace(cfg.Collection.Command)
	default:
		return fmt.Sprintf("zabbix-%s", cfg.Collection.Type)
	}
}

func defaultTimeout(cfg JobConfig) time.Duration {
	if d := cfg.Collection.TimeoutDuration(); d > 0 {
		return d
	}
	return 30 * time.Second
}

type jobCollector struct {
	log  *logger.Logger
	jobs map[string]JobConfig
}

func newJobCollector(cfgs []JobConfig, log *logger.Logger) *jobCollector {
	jobs := make(map[string]JobConfig, len(cfgs))
	for _, cfg := range cfgs {
		jobs[cfg.Name] = cfg
	}
	return &jobCollector{log: log, jobs: jobs}
}

func (c *jobCollector) Run(ctx context.Context, job runtime.JobRuntime, timeout time.Duration) ([]byte, string, ndexec.ResourceUsage, error) {
	cfg, ok := c.jobs[job.Spec.Name]
	if !ok {
		return nil, "", ndexec.ResourceUsage{}, fmt.Errorf("zabbix: unknown job %s", job.Spec.Name)
	}
	macros := buildCollectionMacros(cfg, job)
	switch cfg.Collection.Type {
	case CollectionCommand, "":
		return c.runCommand(ctx, job, cfg, macros, timeout)
	case CollectionHTTP:
		return c.runHTTP(ctx, job, cfg, macros, timeout)
	case CollectionSNMP:
		return c.runSNMP(ctx, job, cfg, macros, timeout)
	default:
		return nil, "", ndexec.ResourceUsage{}, fmt.Errorf("zabbix: unsupported collection type %q", cfg.Collection.Type)
	}
}

func (c *jobCollector) runCommand(ctx context.Context, job runtime.JobRuntime, cfg JobConfig, macros map[string]string, timeout time.Duration) ([]byte, string, ndexec.ResourceUsage, error) {
	cmd := strings.TrimSpace(expandTemplate(cfg.Collection.Command, macros))
	if cmd == "" {
		return nil, "", ndexec.ResourceUsage{}, fmt.Errorf("command is required for command collection")
	}
	var env []string
	for k, v := range expandStringMap(cfg.Collection.Environment, macros) {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	opts := ndexec.RunOptions{Env: env}
	args := expandStringSlice(cfg.Collection.Args, macros)
	data, cmdStr, usage, err := ndexec.RunUnprivilegedWithOptionsUsage(c.log, timeout, opts, cmd, args...)
	return data, cmdStr, usage, err
}

func (c *jobCollector) runHTTP(ctx context.Context, job runtime.JobRuntime, cfg JobConfig, macros map[string]string, timeout time.Duration) ([]byte, string, ndexec.ResourceUsage, error) {
	httpCfg := cfg.Collection.HTTP
	if httpCfg.URL == "" {
		httpCfg.URL = cfg.Collection.URL
	}
	if httpCfg.URL == "" {
		return nil, "", ndexec.ResourceUsage{}, fmt.Errorf("http.url is required")
	}
	method := strings.ToUpper(strings.TrimSpace(httpCfg.Method))
	if method == "" {
		method = strings.ToUpper(strings.TrimSpace(cfg.Collection.Method))
	}
	if method == "" {
		method = "GET"
	}
	urlStr := expandTemplate(httpCfg.URL, macros)
	var body io.Reader
	if httpCfg.Body != "" {
		body = strings.NewReader(expandTemplate(httpCfg.Body, macros))
	} else if cfg.Collection.Body != "" {
		body = strings.NewReader(expandTemplate(cfg.Collection.Body, macros))
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, "", ndexec.ResourceUsage{}, err
	}
	headers := mergeStringMap(cfg.Collection.Headers, httpCfg.Headers)
	for k, v := range headers {
		req.Header.Set(k, expandTemplate(v, macros))
	}
	if httpCfg.Username != "" {
		user := expandTemplate(httpCfg.Username, macros)
		pass := expandTemplate(httpCfg.Password, macros)
		req.SetBasicAuth(user, pass)
	}
	if httpCfg.TLS {
		if req.URL.Scheme == "" {
			req.URL.Scheme = "https"
		} else if req.URL.Scheme != "https" {
			req.URL.Scheme = "https"
		}
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", ndexec.ResourceUsage{}, err
	}
	defer resp.Body.Close()
	buf := &bytes.Buffer{}
	if _, err := io.Copy(buf, resp.Body); err != nil {
		return nil, "", ndexec.ResourceUsage{}, err
	}
	return buf.Bytes(), fmt.Sprintf("http %s", req.URL.String()), ndexec.ResourceUsage{}, nil
}

func (c *jobCollector) runSNMP(ctx context.Context, job runtime.JobRuntime, cfg JobConfig, macros map[string]string, timeout time.Duration) ([]byte, string, ndexec.ResourceUsage, error) {
	snmpCfg := cfg.Collection.SNMP
	target := strings.TrimSpace(expandTemplate(snmpCfg.Target, macros))
	oid := strings.TrimSpace(expandTemplate(snmpCfg.OID, macros))
	if target == "" || oid == "" {
		return nil, "", ndexec.ResourceUsage{}, fmt.Errorf("snmp.target and snmp.oid are required")
	}
	snmpClient := &gosnmp.GoSNMP{
		Target:    target,
		Port:      161,
		Timeout:   timeout,
		Community: expandTemplate(snmpCfg.Community, macros),
		MaxOids:   1,
	}
	if val := strings.TrimSpace(snmpCfg.Context); val != "" {
		snmpClient.ContextName = expandTemplate(val, macros)
	}
	switch strings.TrimSpace(strings.ToLower(snmpCfg.Version)) {
	case "3", "v3":
		snmpClient.Version = gosnmp.Version3
		security := &gosnmp.UsmSecurityParameters{
			UserName:                 expandTemplate(snmpCfg.User, macros),
			AuthenticationPassphrase: expandTemplate(snmpCfg.AuthPass, macros),
			PrivacyPassphrase:        expandTemplate(snmpCfg.PrivPass, macros),
		}
		if strings.EqualFold(snmpCfg.AuthProto, "sha") {
			security.AuthenticationProtocol = gosnmp.SHA
		} else {
			security.AuthenticationProtocol = gosnmp.MD5
		}
		if strings.EqualFold(snmpCfg.PrivProto, "aes") {
			security.PrivacyProtocol = gosnmp.AES
		} else {
			security.PrivacyProtocol = gosnmp.DES
		}
		snmpClient.SecurityParameters = security
		snmpClient.SecurityModel = gosnmp.UserSecurityModel
		snmpClient.MsgFlags = gosnmp.AuthPriv
	case "1", "v1":
		snmpClient.Version = gosnmp.Version1
	default:
		snmpClient.Version = gosnmp.Version2c
	}
	if err := snmpClient.Connect(); err != nil {
		return nil, "", ndexec.ResourceUsage{}, err
	}
	defer snmpClient.Conn.Close()
	if requiresSNMPWalk(cfg) {
		entries, err := snmpClient.BulkWalkAll(oid)
		if err != nil {
			return nil, "", ndexec.ResourceUsage{}, err
		}
		if len(entries) == 0 {
			return nil, "", ndexec.ResourceUsage{}, fmt.Errorf("snmp: walk returned no data")
		}
		payload := formatSNMPWalk(entries)
		return []byte(payload), fmt.Sprintf("snmp %s %s walk", target, oid), ndexec.ResourceUsage{}, nil
	}
	result, err := snmpClient.Get([]string{oid})
	if err != nil {
		return nil, "", ndexec.ResourceUsage{}, err
	}
	if len(result.Variables) == 0 {
		return nil, "", ndexec.ResourceUsage{}, fmt.Errorf("snmp: no variables returned")
	}
	val := formatSNMPValue(result.Variables[0])
	return []byte(val), fmt.Sprintf("snmp %s %s", target, oid), ndexec.ResourceUsage{}, nil
}

func mergeStringMap(base map[string]string, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}

func formatSNMPValue(pdu gosnmp.SnmpPDU) string {
	switch v := pdu.Value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int, uint, uint32, uint64, int64:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatSNMPWalk(vars []gosnmp.SnmpPDU) string {
	var b strings.Builder
	for i, entry := range vars {
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s = %s: %s", entry.Name, snmpTypeName(entry.Type), formatSNMPValue(entry))
	}
	return b.String()
}

func snmpTypeName(kind gosnmp.Asn1BER) string {
	switch kind {
	case gosnmp.OctetString:
		return "STRING"
	case gosnmp.Integer:
		return "INTEGER"
	case gosnmp.Counter32:
		return "Counter32"
	case gosnmp.Gauge32:
		return "Gauge32"
	case gosnmp.IPAddress:
		return "IpAddress"
	case gosnmp.TimeTicks:
		return "Timeticks"
	case gosnmp.Counter64:
		return "Counter64"
	case gosnmp.ObjectIdentifier:
		return "OID"
	case gosnmp.Opaque:
		return "Opaque"
	default:
		return "Unknown"
	}
}

func requiresSNMPWalk(cfg JobConfig) bool {
	for _, pipe := range cfg.Pipelines {
		for _, step := range pipe.Steps {
			switch step.Type {
			case zpre.StepTypeSNMPWalkValue, zpre.StepTypeSNMPWalkToJSON, zpre.StepTypeSNMPWalkToJSONMulti:
				return true
			}
		}
	}
	return false
}

func buildCollectionMacros(cfg JobConfig, job runtime.JobRuntime) map[string]string {
	macros := make(map[string]string)
	host := firstNonEmpty(strings.TrimSpace(job.Vnode.Hostname), strings.TrimSpace(job.Spec.Vnode), strings.TrimSpace(cfg.Vnode), cfg.Name)
	if host == "" {
		host = cfg.Name
	}
	alias := firstNonEmpty(strings.TrimSpace(job.Vnode.Alias), host)
	ip := strings.TrimSpace(job.Vnode.Address)
	conn := firstNonEmpty(ip, host)
	macros["{HOST.NAME}"] = host
	macros["{HOST.HOST}"] = host
	macros["{HOST.DNS}"] = host
	macros["{HOST.ALIAS}"] = alias
	macros["{HOST.IP}"] = ip
	macros["{HOST.CONN}"] = conn
	macros["{ITEM.NAME}"] = cfg.Name
	macros["{ITEM.ID}"] = cfg.Name
	macros["{ITEM.KEY}"] = pluginName(cfg)
	for k, v := range cfg.UserMacros {
		key := strings.TrimSpace(strings.ToUpper(k))
		if key == "" {
			continue
		}
		val := v
		switch {
		case strings.HasPrefix(key, "{$") && strings.HasSuffix(key, "}"):
			macros[key] = val
		case strings.HasPrefix(key, "$") && strings.HasSuffix(key, "$"):
			macros[key] = val
		default:
			macros[fmt.Sprintf("$%s$", key)] = val
			macros[fmt.Sprintf("{$%s}", key)] = val
		}
	}
	return macros
}

func expandTemplate(tmpl string, macros map[string]string) string {
	if tmpl == "" || len(macros) == 0 {
		return tmpl
	}
	res := tmpl
	for k, v := range macros {
		res = strings.ReplaceAll(res, k, v)
	}
	return res
}

func expandStringSlice(values []string, macros map[string]string) []string {
	if len(values) == 0 {
		return values
	}
	res := make([]string, len(values))
	for i, v := range values {
		res[i] = expandTemplate(v, macros)
	}
	return res
}

func expandStringMap(values map[string]string, macros map[string]string) map[string]string {
	if len(values) == 0 {
		return values
	}
	res := make(map[string]string, len(values))
	for k, v := range values {
		res[k] = expandTemplate(v, macros)
	}
	return res
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
