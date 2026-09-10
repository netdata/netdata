// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ddprofiledefinition

import (
	"errors"
	"fmt"
)

func validateConsumers(path string, consumers ConsumerSet) error {
	var errs []error
	seen := make(map[ProfileConsumer]int)
	for i, consumer := range consumers {
		switch consumer {
		case ConsumerMetrics, ConsumerTopology, ConsumerLicensing, ConsumerBGP:
		default:
			errs = append(errs, fmt.Errorf("%s[%d]: invalid consumer %q", path, i, consumer))
			continue
		}
		if firstIdx, ok := seen[consumer]; ok {
			errs = append(
				errs,
				fmt.Errorf("%s[%d]: duplicate consumer %q (first occurrence at index %d)", path, i, consumer, firstIdx),
			)
			continue
		}
		seen[consumer] = i
	}
	return errors.Join(errs...)
}
