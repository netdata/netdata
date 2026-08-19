// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"

func (c *Collector) profileRelabelAndAssemble(
	batch prompkg.SampleBatch,
	normalizers []profileNormalizer,
	checking bool,
) (prompkg.MetricFamilies, error) {
	normalizerIndexByFamily := selectProfileNormalizers(batch, normalizers)

	t := newRelabelTracking()
	help := newHelpRemap()
	processed := prompkg.SampleBatch{Samples: make([]prompkg.Sample, 0, len(batch.Samples))}
	for _, raw := range batch.Samples {
		source := helpFamilyName(raw)
		result := relabelResult{raw: raw, sample: raw}
		if idx, ok := normalizerIndexByFamily[source]; ok {
			normalizer := normalizers[idx]
			result.sample, result.drop = c.applyObservedPipeline(
				raw, normalizer.pipeline, profileRelabelStage, normalizer.name, false,
			)
		}
		c.appendRelabelResult(&processed, t, help, profileRelabelStage, result)
	}
	processed.Help = help.remap(batch.Help)

	_, mfs, err := c.finishRelabel(processed, t, profileRelabelStage, checking, true)
	return mfs, err
}

// selectProfileNormalizers assigns each original source family to the first
// applicable profile normalizer. Applicability is evaluated only from original
// names, so profile pipelines never chain through names produced by one another.
func selectProfileNormalizers(batch prompkg.SampleBatch, normalizers []profileNormalizer) map[string]int {
	type sourceName struct {
		family   string
		physical string
	}
	type checkedSource struct {
		name    sourceName
		matched int
		valid   bool
	}

	selected := make(map[string]int)
	// Labeled series for one physical metric normally arrive consecutively. Cache
	// only that name: a family-level miss would be wrong when a later component
	// (for example _sum after _bucket) makes a profile applicable.
	var last checkedSource
	for _, sample := range batch.Samples {
		base := helpFamilyName(sample)
		limit := len(normalizers)
		if idx, ok := selected[base]; ok {
			limit = idx
		}
		if limit == 0 {
			continue
		}

		key := sourceName{family: base, physical: sample.Name}
		if last.valid && last.name == key {
			if last.matched < limit {
				selected[base] = last.matched
			}
			continue
		}

		matched := limit
		for i := range normalizers[:limit] {
			normalizer := &normalizers[i]
			if !normalizer.root.MatchString(base) {
				continue
			}
			if normalizer.pipeline.Matches(sample.Name) {
				matched = i
				break
			}
		}
		if matched < limit {
			selected[base] = matched
		}
		last = checkedSource{name: key, matched: matched, valid: true}
	}
	return selected
}
