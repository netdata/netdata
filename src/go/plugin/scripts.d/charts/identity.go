// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/ids"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

// JobIdentity encapsulates every bit of information needed to build stable chart IDs,
// labels, and titles for a specific Nagios job execution.
type JobIdentity struct {
	Scheduler   string
	JobName     string
	JobKey      string
	PluginBase  string
	ChartKey    string
	ScriptKey   string
	ScriptTitle string
	Cmdline     string
}

// NewJobIdentity builds a deterministic identifier for the given job under the
// provided scheduler. The resulting ChartKey is stable across restarts and unique for
// a specific script + parameter combination (vnode is intentionally excluded).
func NewJobIdentity(scheduler string, job spec.JobSpec) JobIdentity {
	cmdline := buildCmdline(job)
	pluginBase := pluginBasename(job)
	scriptKey, scriptTitle := scriptKeyForJob(job, pluginBase)
	chartKey := chartKeyForJob(job, cmdline, scriptKey)
	jobKey := jobKeyForJob(job)

	return JobIdentity{
		Scheduler:   scheduler,
		JobName:     job.Name,
		JobKey:      jobKey,
		PluginBase:  pluginBase,
		ChartKey:    chartKey,
		ScriptKey:   scriptKey,
		ScriptTitle: scriptTitle,
		Cmdline:     cmdline,
	}
}

// Labels returns the canonical label set shared across every chart for this job.
func (id JobIdentity) Labels() []module.Label {
	return []module.Label{
		{Key: "nagios_job", Value: id.JobName, Source: module.LabelSourceConf},
		{Key: "nagios_scheduler", Value: id.Scheduler, Source: module.LabelSourceConf},
		{Key: "nagios_plugin", Value: id.PluginBase, Source: module.LabelSourceConf},
		{Key: "nagios_cmdline", Value: id.Cmdline, Source: module.LabelSourceConf},
	}
}

func buildCmdline(job spec.JobSpec) string {
	parts := append([]string{strings.TrimSpace(job.Plugin)}, job.Args...)
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		filtered = append(filtered, part)
	}
	return strings.Join(filtered, " ")
}

func scriptKeyForJob(job spec.JobSpec, pluginBase string) (string, string) {
	base := pluginBase
	if base == "" {
		base = job.Name
	}
	base = strings.TrimSuffix(base, filepath.Ext(base))
	sanitized := ids.Sanitize(base)
	if sanitized == "" {
		sanitized = "nagios_job"
	}
	scriptTitle := strings.ReplaceAll(sanitized, "_", " ")
	return sanitized, scriptTitle
}

func pluginBasename(job spec.JobSpec) string {
	base := filepath.Base(strings.TrimSpace(job.Plugin))
	if base == "" {
		return job.Name
	}
	return base
}

func chartKeyForJob(job spec.JobSpec, cmdline, scriptKey string) string {
	snippet := sanitizeArgs(scriptKey, job.Args)
	signature := buildJobSignature(job, cmdline)
	hash := shortHash(signature)
	key := strings.Trim(strings.Join([]string{snippet, hash}, "_"), "_")
	if key == "" {
		key = hash
	}
	return ids.Sanitize(key)
}

func jobKeyForJob(job spec.JobSpec) string {
	key := ids.Sanitize(job.Name)
	if key == "" {
		key = "job"
	}
	return key
}

func sanitizeArgs(scriptKey string, args []string) string {
	parts := []string{scriptKey}
	for _, arg := range args {
		if arg == "" {
			continue
		}
		parts = append(parts, ids.Sanitize(arg))
	}
	return strings.Trim(strings.Join(parts, "_"), "_")
}

func buildJobSignature(job spec.JobSpec, cmdline string) string {
	var b strings.Builder
	b.WriteString(cmdline)
	b.WriteByte('|')
	b.WriteString(job.WorkingDirectory)
	b.WriteByte('|')
	appendMap(&b, job.Environment)
	b.WriteByte('|')
	appendMap(&b, job.CustomVars)
	b.WriteByte('|')
	appendSlice(&b, job.ArgValues)
	return b.String()
}

func appendMap(b *strings.Builder, m map[string]string) {
	if len(m) == 0 {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(m[k])
		b.WriteByte(';')
	}
}

func appendSlice(b *strings.Builder, values []string) {
	for _, v := range values {
		if v == "" {
			continue
		}
		b.WriteString(v)
		b.WriteByte(';')
	}
}

func shortHash(data string) string {
	sum := sha1.Sum([]byte(data))
	return hex.EncodeToString(sum[:4])
}

func (id JobIdentity) TelemetryChartID(metric string) string {
	return fmt.Sprintf("%s.%s.%s.%s", ctxPrefix, id.Scheduler, id.JobKey, metric)
}

func (id JobIdentity) TelemetryMetricID(metric, dim string) string {
	return fmt.Sprintf("%s.%s", id.TelemetryChartID(metric), dim)
}

func (id JobIdentity) PerfdataChartID(labelID string) string {
	return fmt.Sprintf("%s.%s.%s.perf_%s", ctxPrefix, id.Scheduler, id.JobKey, labelID)
}

func (id JobIdentity) PerfdataMetricID(labelID, dim string) string {
	return fmt.Sprintf("%s.%s", id.PerfdataChartID(labelID), dim)
}

func (id JobIdentity) MetricPrefix() string {
	return fmt.Sprintf("%s.%s.%s.", ctxPrefix, id.Scheduler, id.JobKey)
}

func (id JobIdentity) OwnsMetric(key string) bool {
	return strings.HasPrefix(key, id.MetricPrefix())
}
