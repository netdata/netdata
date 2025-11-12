// SPDX-License-Identifier: GPL-3.0-or-later

package charts

import (
	"crypto/sha1"
	"encoding/hex"
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
	Shard       string
	JobName     string
	ChartKey    string
	ScriptKey   string
	ScriptTitle string
	Cmdline     string
}

// NewJobIdentity builds a deterministic identifier for the given job under the
// provided shard. The resulting ChartKey is stable across restarts and unique for
// a specific script + parameter combination (vnode is intentionally excluded).
func NewJobIdentity(shard string, job spec.JobSpec) JobIdentity {
	cmdline := buildCmdline(job)
	scriptKey, scriptTitle := scriptKeyForJob(job)
	chartKey := chartKeyForJob(job, cmdline, scriptKey)

	return JobIdentity{
		Shard:       shard,
		JobName:     job.Name,
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
		{Key: "nagios_shard", Value: id.Shard, Source: module.LabelSourceConf},
		{Key: "nagios_cmdline", Value: id.Cmdline, Source: module.LabelSourceConf},
	}
}

// ChartID formats the canonical chart ID for a measurement suffix.
func (id JobIdentity) ChartID(suffix string) string {
	return ChartIDFromParts(id.Shard, id.ChartKey, suffix)
}

// MetricID builds the name used in the metrics map for a dimension.
func (id JobIdentity) MetricID(suffix, dim string) string {
	return MetricKeyFromParts(id.Shard, id.ChartKey, suffix, dim)
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

func scriptKeyForJob(job spec.JobSpec) (string, string) {
	base := filepath.Base(strings.TrimSpace(job.Plugin))
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
