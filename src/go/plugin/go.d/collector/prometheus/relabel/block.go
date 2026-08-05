// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

// Block scopes an ordered list of metric relabel rules to matching metric names.
// Blocks themselves run in list order and match the sample name produced by the
// preceding block.
type Block struct {
	Match                string   `yaml:"match" json:"match"`
	MetricRelabelConfigs []Config `yaml:"metric_relabel_configs" json:"metric_relabel_configs"`
}

// CloneBlocks returns an ownership-independent copy of blocks, including rule
// slices and compiled regular expressions.
func CloneBlocks(blocks []Block) []Block {
	if blocks == nil {
		return nil
	}
	out := make([]Block, len(blocks))
	for i, block := range blocks {
		out[i] = block
		if block.MetricRelabelConfigs != nil {
			out[i].MetricRelabelConfigs = make([]Config, len(block.MetricRelabelConfigs))
			for j, cfg := range block.MetricRelabelConfigs {
				out[i].MetricRelabelConfigs[j] = cfg.clone()
			}
		}
	}
	return out
}

type compiledBlock struct {
	match matcher.Matcher
	proc  *Processor
}

// Pipeline is a compiled block list. Its processors reuse buffers, so a Pipeline
// belongs to one collector job and is not goroutine-safe.
type Pipeline struct {
	blocks []compiledBlock
}

// NewPipeline validates and compiles relabel blocks. An empty block list returns
// a nil pipeline so callers can preserve their no-relabel fast path.
func NewPipeline(blocks []Block) (*Pipeline, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	compiled := make([]compiledBlock, 0, len(blocks))
	for i, block := range blocks {
		if strings.TrimSpace(block.Match) == "" {
			return nil, fmt.Errorf("relabeling[%d]: 'match' is required", i)
		}
		if len(block.MetricRelabelConfigs) == 0 {
			return nil, fmt.Errorf("relabeling[%d]: 'metric_relabel_configs' is empty", i)
		}

		m, err := matcher.NewSimplePatternsMatcher(block.Match)
		if err != nil {
			return nil, fmt.Errorf("relabeling[%d]: invalid 'match' (%q): %v", i, block.Match, err)
		}
		proc, err := New(block.MetricRelabelConfigs)
		if err != nil {
			return nil, fmt.Errorf("relabeling[%d]: %v", i, err)
		}
		compiled = append(compiled, compiledBlock{match: m, proc: proc})
	}

	return &Pipeline{blocks: compiled}, nil
}

// Matches reports whether any block matcher accepts name. It does not execute
// rules and is safe for determining which selected profile owns an input family.
func (p *Pipeline) Matches(name string) bool {
	if p == nil {
		return false
	}
	for i := range p.blocks {
		if p.blocks[i].match.MatchString(name) {
			return true
		}
	}
	return false
}

// Apply runs matching blocks in order. Each block matches the current sample
// name, including a name produced by an earlier block.
func (p *Pipeline) Apply(sample prompkg.Sample) (prompkg.Sample, DropInfo) {
	if p == nil {
		return sample, DropInfo{}
	}
	for i := range p.blocks {
		block := &p.blocks[i]
		if !block.match.MatchString(sample.Name) {
			continue
		}
		out, drop := block.proc.Apply(sample)
		if drop.Dropped() {
			return sample, drop
		}
		sample = out
	}
	return sample, DropInfo{}
}

// ApplyWithObserver runs the same pipeline as Apply and reports entered blocks
// and evaluated rules. It is intended for opt-in validation diagnostics.
func (p *Pipeline) ApplyWithObserver(
	sample prompkg.Sample,
	observeBlock BlockDiagnosticObserver,
	observeRule PipelineRuleDiagnosticObserver,
) (prompkg.Sample, DropInfo) {
	if p == nil {
		return sample, DropInfo{}
	}
	for i := range p.blocks {
		block := &p.blocks[i]
		if !block.match.MatchString(sample.Name) {
			continue
		}
		if observeBlock != nil {
			labelNames := make([]string, 0, len(sample.Labels))
			for _, label := range sample.Labels {
				labelNames = append(labelNames, label.Name)
			}
			observeBlock(BlockDiagnostic{
				BlockIndex:      i,
				InputMetricName: sample.Name,
				InputLabelNames: labelNames,
			})
		}
		out, drop := block.proc.ApplyWithObserver(sample, func(fact RuleDiagnostic) {
			if observeRule != nil {
				observeRule(i, fact)
			}
		})
		if drop.Dropped() {
			return sample, drop
		}
		sample = out
	}
	return sample, DropInfo{}
}
