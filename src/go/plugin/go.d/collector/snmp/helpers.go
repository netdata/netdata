package snmp

func sliceToStrings(items []interface{}) []string {
	var strs []string
	for _, v := range items {
		s, ok := v.(string)
		if !ok {
			// Handle error if an element is not a string.
			continue
		}
		strs = append(strs, s)
	}
	return strs
}

func sliceToTableMetricTags(items []interface{}) []TableMetricTag {
	var metricTag []TableMetricTag
	for _, v := range items {
		s, ok := v.(TableMetricTag)
		if !ok {
			// Handle error if an element is not a string.
			continue
		}
		metricTag = append(metricTag, s)
	}
	return metricTag
}

func mergeTableBatches(target tableBatches, source tableBatches) tableBatches {
	merged := tableBatches{}

	// Extend batches in `target` with OIDs from `source` that share the same key.
	for key, batch := range target {

		if srcBatch, ok := source[key]; ok {
			mergedOids := append(batch.oids, srcBatch.oids...)
			merged[key] = tableBatch{
				tableOID: batch.tableOID,
				oids:     mergedOids,
			}
		}
	}

	for key := range source {
		if _, ok := target[key]; !ok {
			merged[key] = source[key]
		}
	}

	return merged
}

func mergeStringMaps(m1 map[string]string, m2 map[string]string) map[string]string {
	merged := make(map[string]string)
	for k, v := range m1 {
		merged[k] = v
	}
	for key, value := range m2 {
		merged[key] = value
	}
	return merged
}

func mergeProcessedMetricMaps(m1 map[string]processedMetric, m2 map[string]processedMetric) map[string]processedMetric {
	merged := make(map[string]processedMetric)
	for k, v := range m1 {
		merged[k] = v
	}
	for key, value := range m2 {
		merged[key] = value
	}
	return merged
}
