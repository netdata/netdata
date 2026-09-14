// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"testing"
)

func TestProfileMetricDiagnosticChartUsesTemplateLocalContext(t *testing.T) {
	chart := profileMetricDiagnosticChart()
	if chart.Context != "profile_metric_diagnostics" {
		t.Fatalf("diagnostic chart context = %q, want profile_metric_diagnostics", chart.Context)
	}
}
