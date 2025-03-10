package snmp

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"regexp"
)

func parseMetrics(metrics []Metric) (parsedResult, error) {
	oids := []string{}
	next_oids := []string{}
	bulk_oids := []string{}
	parsed_metrics := []parsedMetric{}
	oids_to_resolve := []map[string]string{}
	indexes_to_resolve := []indexMapping{}
	bulk_threshold := 0
	for _, metric := range metrics {
		result, err := parseMetric(metric)

		if err != nil {
			return parsedResult{}, err
		}

		oids = append(oids, result.oidsToFetch...)

		for name, oid := range result.oidsToResolve {
			// here in the python implementation a registration happens to their OIDResolver. I will not support this atm
			oids_to_resolve = append(oids_to_resolve, map[string]string{name: oid})
		}

		// here in the python implementation a registration happens to their OIDResolver. I will not support this atm
		indexes_to_resolve = append(indexes_to_resolve, result.indexMappings...)

		for _, batch := range result.tableBatches {
			should_query_in_bulk := bulk_threshold > 0 && len(batch.oids) > bulk_threshold
			if should_query_in_bulk {
				bulk_oids = append(bulk_oids, batch.tableOID)
			} else {
				next_oids = append(next_oids, batch.oids...)
			}
		}

		parsed_metrics = append(parsed_metrics, result.parsedMetrics...)

	}
	return parsedResult{oids: oids,
		next_oids: next_oids, bulk_oids: bulk_oids, parsed_metrics: parsed_metrics}, nil
}

func parseMetric(metric Metric) (metricParseResult, error) {
	/*Can either be:

	* An OID metric:

	```
	metrics:
	  - OID: 1.3.6.1.2.1.2.2.1.14
	    name: ifInErrors
	```

	* A symbol metric:

	```
	metrics:
	  - MIB: IF-MIB
	    symbol: ifInErrors
	    # OR:
	    symbol:
	      OID: 1.3.6.1.2.1.2.2.1.14
	      name: ifInErrors
	```

	* A table metric (see parsing for table metrics for all possible options):

	```
	metrics:
	  - MIB: IF-MIB
	    table: ifTable
	    symbols:
	      - OID: 1.3.6.1.2.1.2.2.1.14
	        name: ifInErrors
	```*/

	// Cast the tags to either array
	castedStringMetricTags := []string{}
	castedTableMetricTags := []TableMetricTag{}

	if len(metric.MetricTags) > 0 {
		/*//TODO investigate if there are metric tags and not only table metric tags
		fmt.Println("parseMetric MetricTags switch, have:")
		spew.Dump(metric.MetricTags)
		switch metric.MetricTags[0].(type) {
		case string:
			os.Exit(-40)
			castedStringMetricTags = sliceToStrings(metric.MetricTags)

		case MetricTag:
			os.Exit(-40)
			castedTableMetricTags = sliceToTableMetricTags(metric.MetricTags)
		}*/
	}

	if len(metric.OID) > 0 {
		// TODO investigate if this exists in the yamls
		return (parseOIDMetric(oidMetric{name: metric.Name, oid: metric.OID, metricTags: castedStringMetricTags, forcedType: metric.MetricType, options: metric.Options})), nil
	} else if len(metric.MIB) == 0 {
		return metricParseResult{}, fmt.Errorf("unsupported metric {%v}", metric)
	} else if metric.Symbol != (Symbol{}) {
		// Single Metric
		return (parseSymbolMetric(symbolMetric{mib: metric.MIB, symbol: metric.Symbol, forcedType: metric.MetricType, metricTags: castedStringMetricTags, options: metric.Options}))
	} else if metric.Table != nil {
		// Table
		if len(metric.MetricTags) > 0 {

			/*//This will be cleared out when tags are supported
			// for _, rawItem := range metric.MetricTags {
			// 	item, ok := rawItem.(map[string]interface{})
			// 	if !ok {
			// 		continue
			// 	}
			// 	fmt.Println("ITEM", item["symbol"])

			// 	var index int
			// 	if val, exists := item["Index"]; exists {
			// 		if i, ok := val.(int); ok {
			// 			index = i
			// 		} else {
			// 			index = -1
			// 		}
			// 	}

			// 	var mapping map[int]string
			// 	if val, exists := item["mapping"]; exists {
			// 		if m, ok := val.(map[int]string); ok {
			// 			fmt.Print("IN")
			// 			os.Exit(-90)
			// 			mapping = m
			// 		}
			// 	}

			// 	var tag string
			// 	if val, exists := item["Tag"]; exists {
			// 		if s, ok := val.(string); ok {
			// 			tag = s
			// 		}
			// 	}

			// 	var symbol Symbol
			// 	if rawSymbol, exists := item["symbol"]; exists {
			// 		if symMap, ok := rawSymbol.(map[string]interface{}); ok {
			// 			var oid, name string

			// 			if v, exists := symMap["OID"]; exists {
			// 				if oidStr, ok := v.(string); ok {
			// 					oid = oidStr
			// 				}
			// 			}

			// 			if v, exists := symMap["name"]; exists {
			// 				if nameStr, ok := v.(string); ok {
			// 					name = nameStr
			// 				}
			// 			}

			// 			symbol = Symbol{OID: oid, Name: name}
			// 		} else {
			// 			fmt.Println("symbol is not a map[string]interface{}")
			// 		}
			// 	}

			// 	var table string
			// 	if val, exists := item["Table"]; exists {
			// 		if s, ok := val.(string); ok {
			// 			table = s
			// 		} else {
			// 			table = ""
			// 		}
			// 	}

			// 	var mib string
			// 	if val, exists := item["MIB"]; exists {
			// 		if s, ok := val.(string); ok {
			// 			mib = s
			// 		} else {
			// 			mib = ""
			// 		}
			// 	}

			// 	var indexTransform []IndexSlice
			// 	if val, exists := item["IndexTransform"]; exists {
			// 		if xs, ok := val.([]IndexSlice); ok {
			// 			indexTransform = xs
			// 		}
			// 	}*/

			castedTableMetricTags = append(castedTableMetricTags, metric.MetricTags...)
		}

		if metric.Symbols == nil {
			return metricParseResult{}, fmt.Errorf("when specifying a table, you must specify a list of symbols %v", metric)
		}

		return (parseTableMetric(tableMetric{
			mib:        metric.MIB,
			table:      metric.Table,
			symbols:    metric.Symbols,
			forcedType: metric.MetricType,
			metricTags: castedTableMetricTags,
			options:    metric.Options,
		}))

	}
	return metricParseResult{}, fmt.Errorf("unsupported metric {%v}", metric)
}

// TODO error outs on functions
func parseOIDMetric(metric oidMetric) metricParseResult {
	/*Parse a fully resolved OID/name metric.

	  Note: This `OID/name` syntax is deprecated in favour of `symbol` syntax.

	  Example:

	  ```
	  metrics:
	    - OID: 1.3.6.1.2.1.2.1
	      name: ifNumber
	  ```
	*/
	name := metric.name
	oid := metric.oid

	// TODO can't find a profile with this metric type

	parsed_symbol_metric := parsedSymbolMetric{
		name:          name,
		tags:          metric.metricTags,
		forcedType:    metric.forcedType,
		enforceScalar: true,
		options:       metric.options,
		baseoid:       oid,
	}

	return metricParseResult{
		oidsToFetch:   []string{oid},
		oidsToResolve: map[string]string{name: oid},
		parsedMetrics: []parsedMetric{parsed_symbol_metric},
		tableBatches:  nil,
		indexMappings: nil,
	}
}

// TODO error outs on functions
func parseSymbolMetric(metric symbolMetric) (metricParseResult, error) {
	/*    Parse a symbol metric (= an OID in a MIB).
	Example:

	```
	metrics:
	  - MIB: IF-MIB
	    symbol: <string or OID/name object>
	  - MIB: IF-MIB
	    symbol:                     # MIB-less syntax
	      OID: 1.3.6.1.2.1.6.5.0
	      name: tcpActiveOpens
	  - MIB: IF-MIB
	    symbol: tcpActiveOpens      # require MIB syntax
	```*/
	mib := metric.mib
	symbol := metric.symbol
	parsed_symbol, err := parseSymbol(mib, symbol)
	if err != nil {
		return metricParseResult{}, err
	}

	parsed_symbol_metric := parsedSymbolMetric{
		name:                parsed_symbol.name,
		tags:                metric.metricTags,
		forcedType:          metric.forcedType,
		enforceScalar:       false,
		options:             metric.options,
		extractValuePattern: parsed_symbol.extractValuePattern,
		baseoid:             parsed_symbol.oid,
	}

	return metricParseResult{
		oidsToFetch:   []string{parsed_symbol.oid},
		oidsToResolve: parsed_symbol.oidsToResolve,
		parsedMetrics: []parsedMetric{parsed_symbol_metric},
		tableBatches:  nil,
		indexMappings: nil,
	}, nil
}

// TODO error outs on functions
func parseTableMetric(metric tableMetric) (metricParseResult, error) {

	mib := metric.mib
	parsed_table, err := parseSymbol(mib, metric.table)
	if err != nil {
		return metricParseResult{}, err
	}

	table_name := parsed_table.name
	table_oid := parsed_table.oid

	oids_to_resolve := parsed_table.oidsToResolve

	var index_tags []indexTag
	var column_tags []columnTag
	var index_mappings []indexMapping
	var table_batches map[tableBatchKey]tableBatch

	if metric.metricTags != nil {
		for _, metric_tag := range metric.metricTags {
			parsed_table_metric_tag, err := parseTableMetricTag(mib, parsed_table, metric_tag)
			if err != nil {
				return metricParseResult{}, err
			}

			if parsed_table_metric_tag.oidsToResolve != nil {
				oids_to_resolve = mergeStringMaps(oids_to_resolve, parsed_table_metric_tag.oidsToResolve)

				column_tags = append(column_tags, parsed_table_metric_tag.columnTags...)

				table_batches = mergeTableBatches(table_batches, parsed_table_metric_tag.tableBatches)
			} else {

				index_tags = append(index_tags, parsed_table_metric_tag.indexTags...)

				for index, mapping := range parsed_table_metric_tag.indexMappings {
					for _, symbol := range metric.symbols {
						index_mappings = append(index_mappings, indexMapping{tag: symbol.Name, index: index, mapping: mapping})
					}

					for _, tag := range metric.metricTags {
						if reflect.DeepEqual(tag.Symbol, Symbol{}) {
							tag = TableMetricTag{
								Tag:    tag.Tag,
								Symbol: tag.Symbol,
							}
							index_mappings = append(index_mappings, indexMapping{
								tag:     tag.Symbol.Name,
								index:   index,
								mapping: mapping,
							})
						}
					}
				}
			}
		}
	}

	table_oids := []string{}
	parsed_metrics := []parsedMetric{}

	for _, symbol := range metric.symbols {
		parsed_symbol, err := parseSymbol(mib, symbol)
		if err != nil {
			return metricParseResult{}, nil
		}

		for key, value := range parsed_symbol.oidsToResolve {
			oids_to_resolve[key] = value
		}

		table_oids = append(table_oids, parsed_symbol.oid)

		parsed_table_metric := parsedTableMetric{
			name:                parsed_symbol.name,
			indexTags:           index_tags,
			columnTags:          column_tags,
			forcedType:          metric.forcedType,
			options:             metric.options,
			extractValuePattern: parsed_symbol.extractValuePattern,
			rowOID:              parsed_symbol.oid,
			tableName:           table_name,
			tableOID:            table_oid,
		}

		parsed_metrics = append(parsed_metrics, parsed_table_metric)
	}

	table_batches = mergeTableBatches(table_batches, map[tableBatchKey]tableBatch{{mib: mib, table: parsed_table.name}: {tableOID: parsed_table.oid, oids: table_oids}})

	return metricParseResult{
		oidsToFetch:   []string{},
		oidsToResolve: oids_to_resolve,
		tableBatches:  table_batches,
		indexMappings: index_mappings,
		parsedMetrics: parsed_metrics,
	}, nil
}

func parseTableMetricTag(mib string, parsed_table parsedSymbol, metric_tag TableMetricTag) (parsedTableMetricTag, error) {
	/*
		Parse an item of the `metric_tags` section of a table metric.

		Items can be:

		* A reference to a column in the same table.

		Example using entPhySensorTable in ENTITY-SENSOR-MIB:

		```
		metric_tags:
			- tag: sensor_type
			column: entPhySensorType
			# OR
			column:
			OID: 1.3.6.1.2.1.99.1.1.1.1
			name: entPhySensorType

		```

		* A reference to a column in a different table.

		Example:

		```
		metric_tags:
			- tag: adapter
			table: genericAdaptersAttrTable
			column: adapterName
			# OR
			column:
			OID: 1.3.6.1.4.1.343.2.7.2.2.1.1.1.2
			name: adapterName

		```

		* A reference to an OID by its index in the table entry.

		An optional `mapping` can be used to map index values to human-readable strings.

		Example using ipIfStatsTable in IP-MIB:

		```
		metric_tags:
			- # ipIfStatsIPVersion (1.3.6.1.2.1.4.21.3.1.1)
			tag: ip_version
			index: 1
			mapping:
			0: unknown
			1: ipv4
			2: ipv6
			3: ipv4z
			4: ipv6z
			16: dns
			- # ipIfStatsIfIndex (1.3.6.1.2.1.4.21.3.1.2)
			tag: interface
			index: 2
			```
	*/
	if metric_tag.Symbol != (Symbol{}) {
		metric_tag_mib := metric_tag.MIB

		if metric_tag.Table != "" {
			return parseOtherTableColumnMetricTag(metric_tag_mib, metric_tag.Table, metric_tag)
		}

		if mib != metric_tag_mib && metric_tag_mib != "" {
			return parsedTableMetricTag{}, fmt.Errorf("when tagging from a different MIB, the table must be specified, TABLE_MAME: %s MIB: %s METRIC_TAG_MIB: %s", parsed_table.name, mib, metric_tag_mib)
		}
		return parseColumnMetricTag(mib, parsed_table, metric_tag)
	} else if &metric_tag.Index != nil { //TODO, this is tautological condition, need to return a "-1" on index if it does not exist or something similar
		return parseIndexMetricTag(metric_tag)
	} else {
		return parsedTableMetricTag{}, errors.New("symbol is empty")
	}

}

func parseIndexMetricTag(metric_tag TableMetricTag) (parsedTableMetricTag, error) {
	parsed_metric_tag, err := parseMetricTag(
		MetricTag{
			Tag: metric_tag.Tag,
		})
	if err != nil {
		return parsedTableMetricTag{}, err
	}

	index_tags := []indexTag{{
		parsedMetricTag: parsed_metric_tag,
		index:           metric_tag.Index,
	}}

	index_mappings := map[int]map[int]string{}

	if metric_tag.Mapping != nil {
		index_mappings = map[int]map[int]string{metric_tag.Index: metric_tag.Mapping}
	}

	return parsedTableMetricTag{
		indexTags:     index_tags,
		indexMappings: index_mappings,
	}, nil
}

func parseOtherTableColumnMetricTag(mib string, table string, metric_tag TableMetricTag) (parsedTableMetricTag, error) {
	parsed_table, err := parseSymbol(mib, &table)
	if err != nil {
		return parsedTableMetricTag{}, err
	}
	parsed_metric_tag, err := parseColumnMetricTag(mib, parsed_table, metric_tag)
	if err != nil {
		return parsedTableMetricTag{}, err
	}

	oids_to_resolve := parsed_metric_tag.oidsToResolve
	oids_to_resolve = mergeStringMaps(oids_to_resolve, parsed_table.oidsToResolve)

	return parsedTableMetricTag{
		oidsToResolve: oids_to_resolve,
		tableBatches:  parsed_metric_tag.tableBatches,
		columnTags:    parsed_metric_tag.columnTags,
	}, nil
}

func parseColumnMetricTag(mib string, parsed_table parsedSymbol, metric_tag TableMetricTag) (parsedTableMetricTag, error) {
	parsed_column, err := parseSymbol(mib, metric_tag.Symbol)
	if err != nil {
		return parsedTableMetricTag{}, err
	}

	batches := map[tableBatchKey]tableBatch{
		{mib: mib, table: parsed_table.name}: {tableOID: parsed_table.oid, oids: []string{parsed_column.oid}},
	}

	parsed_metric_tag, err := parseMetricTag(MetricTag{MIB: metric_tag.MIB, OID: "", Tag: metric_tag.Tag, Symbol: metric_tag.Symbol})
	if err != nil {
		return parsedTableMetricTag{}, err
	}

	return parsedTableMetricTag{
		oidsToResolve: parsed_column.oidsToResolve,
		columnTags: []columnTag{{
			parsedMetricTag: parsed_metric_tag,
			column:          parsed_column.name,
			indexSlices:     parseIndexSlices(metric_tag),
		},
		},
		tableBatches: batches,
	}, nil
}

func parseIndexSlices(metric_tag TableMetricTag) []IndexSlice {
	/*
	   //     Transform index_transform into list of index slices.

	   //     `index_transform` is needed to support tagging using another table with different indexes.

	   //     Example: TableB have two indexes indexX (1 digit) and indexY (3 digits).
	   //         We want to tag by an external TableA that have indexY (3 digits).

	   //         For example TableB has a row with full index `1.2.3.4`, indexX is `1` and indexY is `2.3.4`.
	   //         TableA has a row with full index `2.3.4`, indexY is `2.3.4` (matches indexY of TableB).

	   //         SNMP integration doesn't know how to compare the full indexes from TableB and TableA.
	   //         We need to extract a subset of the full index of TableB to match with TableA full index.

	   //         Using the below `index_transform` we provide enough info to extract a subset of index that
	   //         will be used to match TableA's full index.

	   // ```yaml
	   // index_transform:
	   //   - start: 1
	   //   - end: 3
	   //
	   // ```
	*/
	raw_index_slices := metric_tag.IndexTransform
	index_slices := []IndexSlice{}

	for _, rule := range raw_index_slices {
		start, end := rule.Start, rule.End
		if start > end {
			log.Println("start bigger than end")
			return nil
		}
		if start < 0 {
			log.Println("start is negative")
			return nil
		}
		index_slices = append(index_slices, IndexSlice{start, end + 1})

	}

	return index_slices
}

func parseMetricTag(metric_tag MetricTag) (parsedMetricTag, error) {
	parsed_metric_tag := parsedMetricTag{}

	if metric_tag.Tag != "" {
		parsed_metric_tag = parseSimpleMetricTag(metric_tag)
	} else if metric_tag.Match != "" && metric_tag.Tags != nil {
		tmp, err := parseRegexMetricTag(metric_tag)
		if err != nil {
			return parsedMetricTag{}, err
		} else {
			parsed_metric_tag = tmp
		}
	} else {
		return parsedMetricTag{}, fmt.Errorf("a metric tag must specify either a tag, or a mapping of tags and a regular expression %v", metric_tag)
	}
	return parsed_metric_tag, nil
}

func parseRegexMetricTag(metric_tag MetricTag) (parsedMetricTag, error) {
	match := metric_tag.Match
	tags := metric_tag.Tags

	// To be supported once tags are supported
	// if reflect.TypeOf(tags) != reflect.TypeOf(map[string]string{}) {
	// }

	pattern, err := regexp.Compile(match)
	if err != nil {
		return parsedMetricTag{}, err
	}

	return parsedMetricTag{tags: tags, pattern: pattern}, nil
}

func parseSimpleMetricTag(metric_tag MetricTag) parsedMetricTag {
	return parsedMetricTag{name: metric_tag.Tag}
}

func parseSymbol(mib string, symbol interface{}) (parsedSymbol, error) {
	/*
		Parse an OID symbol.

		This can either be the unresolved name of a symbol:

		```
		symbol: ifNumber
		```

		Or a resolved OID/name object:

		```
		symbol:
		    OID: 1.3.6.1.2.1.2.1
		    name: ifNumber
		```
	*/
	// if reflect.TypeOf(symbol) == reflect.TypeOf(string) {
	// 	// TODO, here they use ObjectIdentity(mib,symbol) to resolve the symbol. this is not straightfowrard in Go, it is a pysnmp function.
	// 	// oid:=

	switch s := symbol.(type) {
	case Symbol:
		oid := s.OID
		name := s.Name
		if s.ExtractValue != "" {
			extractValuePattern, err := regexp.Compile(s.ExtractValue)
			if err != nil {

				return parsedSymbol{}, err
			}
			return parsedSymbol{
				name,
				oid,
				extractValuePattern,
				map[string]string{name: oid},
			}, nil
		} else {
			return parsedSymbol{
				name,
				oid,
				nil,
				map[string]string{name: oid},
			}, nil
		}
	case string:
		return parsedSymbol{}, errors.New("string only symbol, can't support yet")
	case map[string]interface{}:
		oid, okOID := s["OID"].(string)
		name, okName := s["name"].(string)

		if !okOID || !okName {

			return parsedSymbol{}, fmt.Errorf("invalid symbol format: %+v", s)
		}

		return parsedSymbol{
			name:                name,
			oid:                 oid,
			extractValuePattern: nil,
			oidsToResolve:       map[string]string{name: oid},
		}, nil

	default:
		return parsedSymbol{}, fmt.Errorf("unsupported symbol type: %T", symbol)
	}

}
