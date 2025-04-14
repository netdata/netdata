package snmp

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func parseMetrics(metrics []ddprofiledefinition.MetricsConfig) (parsedResult, error) {
	var (
		OIDs, nextOIDs, bulkOIDs []string
		OIDsToResolve            []map[string]string
		parsedMetrics            []parsedMetric
		indexesToResolve         []indexMapping
	)

	bulkThreshold := 0
	for _, metric := range metrics {
		result, err := parseMetric(metric)

		if err != nil {
			return parsedResult{}, err
		}

		OIDs = append(OIDs, result.oidsToFetch...)

		for name, oid := range result.oidsToResolve {
			// here in the python implementation a registration happens to their OIDResolver. I will not support this atm
			OIDsToResolve = append(OIDsToResolve, map[string]string{name: oid})
		}

		// here in the python implementation a registration happens to their OIDResolver. I will not support this atm
		indexesToResolve = append(indexesToResolve, result.indexMappings...)

		for _, batch := range result.tableBatches {
			should_query_in_bulk := bulkThreshold > 0 && len(batch.oids) > bulkThreshold
			if should_query_in_bulk {
				bulkOIDs = append(bulkOIDs, batch.tableOID)
			} else {
				nextOIDs = append(nextOIDs, batch.oids...)
			}
		}

		parsedMetrics = append(parsedMetrics, result.parsedMetrics...)

	}
	return parsedResult{
		OIDs:          OIDs,
		nextOIDs:      nextOIDs,
		bulkOIDs:      bulkOIDs,
		parsedMetrics: parsedMetrics}, nil
}

func parseMetric(metric ddprofiledefinition.MetricsConfig) (metricParseResult, error) {
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

	// Can't support tags at the moment

	if len(metric.OID) > 0 {
		// TODO investigate if this exists in the yamls
		// return (parseOIDMetric(oidMetric{name: metric.Name, oid: metric.OID, metricTags: castedStringMetricTags, forcedType: string(metric.MetricType), options: metric.Options})), nil
		return metricParseResult{}, nil

	} else if len(metric.MIB) == 0 {
		return metricParseResult{}, fmt.Errorf("unsupported metric {%v}", metric)
	} else if metric.Symbol != (ddprofiledefinition.SymbolConfig{}) {
		// Single Metric
		return (parseSymbolMetric(metric.Symbol, metric.MIB)) // TODO metric tags might be needed here.
		//Can't support tables at the moment
	} else {
		return metricParseResult{}, nil
	}
}

// TODO error outs on functions
func parseSymbolMetric(symbol ddprofiledefinition.SymbolConfig, mib string) (metricParseResult, error) {
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

	parsedSymbol, err := parseSymbol(symbol)
	if err != nil {
		return metricParseResult{}, err
	}

	parsedSymbolMetric := parsedSymbolMetric{
		name:                parsedSymbol.name,
		tags:                nil,
		forcedType:          string(symbol.MetricType),
		enforceScalar:       false,
		options:             nil,
		extractValuePattern: parsedSymbol.extractValuePattern,
		baseoid:             parsedSymbol.oid,
		unit:                symbol.Unit,
		description:         symbol.Description,
	}

	return metricParseResult{
		oidsToFetch:   []string{parsedSymbol.oid},
		oidsToResolve: parsedSymbol.oidsToResolve,
		parsedMetrics: []parsedMetric{parsedSymbolMetric},
		tableBatches:  nil,
		indexMappings: nil,
	}, nil
}

func parseSymbol(symbol interface{}) (parsedSymbol, error) {
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

	switch s := symbol.(type) {
	case ddprofiledefinition.SymbolConfig:
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
