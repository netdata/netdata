package wireeval

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/oracle"
)

func TestPerformancePairUsesRealProductionAgent(t *testing.T) {
	baselineExecutable := os.Getenv("NETDATA_JOBMGRTEST_BASELINE")
	productionExecutable := os.Getenv("NETDATA_JOBMGRTEST_PRODUCTION")
	fixtureDirectory := os.Getenv("NETDATA_JOBMGRTEST_FIXTURE")
	if baselineExecutable == "" || productionExecutable == "" || fixtureDirectory == "" {
		t.Skip("set NETDATA_JOBMGRTEST_BASELINE, NETDATA_JOBMGRTEST_PRODUCTION, and NETDATA_JOBMGRTEST_FIXTURE")
	}
	for name, path := range map[string]string{
		"baseline":   baselineExecutable,
		"production": productionExecutable,
		"fixture":    fixtureDirectory,
	} {
		if !filepath.IsAbs(path) {
			t.Fatalf("%s path is not absolute: %s", name, path)
		}
	}
	arguments := []string{
		"--mode=wire/agent",
		"--fixture-config-dir=" + fixtureDirectory,
	}
	baseline := ChildSpec{
		Executable: baselineExecutable,
		Arguments:  append([]string(nil), arguments...),
	}
	production := ChildSpec{
		Executable: productionExecutable,
		Arguments:  append([]string(nil), arguments...),
	}
	environmentSHA256, err := DeriveEnvironmentSHA256(baseline, production)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [16]byte
	nonce[15] = 1
	const workloadID = "B-WL-001-balanced"
	schedule, err := contract.BuildPerformanceSchedule(workloadID, nonce)
	if err != nil {
		t.Fatal(err)
	}
	productionFirst, err := contract.BaselineRunsFirst(0)
	if err != nil {
		t.Fatal(err)
	}
	productionFirst = !productionFirst
	order := []struct {
		side  oracle.Side
		child ChildSpec
		first bool
	}{
		{side: oracle.SideBaseline, child: baseline, first: !productionFirst},
		{side: oracle.SideProduction, child: production, first: productionFirst},
	}
	if productionFirst {
		order[0], order[1] = order[1], order[0]
	}
	summaries := make(map[oracle.Side]oracle.RunSummary, 2)
	for _, member := range order {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		summary, err := Run(ctx, RunSpec{
			Child: member.child, Side: member.side, WorkloadID: workloadID,
			Population: 32, PairIndex: 0, RanFirst: member.first,
			PairNonce: nonce, EnvironmentSHA256: environmentSHA256,
			Schedule: schedule,
		})
		cancel()
		if err != nil {
			t.Fatalf("%s run failed: %v", member.side, err)
		}
		summaries[member.side] = summary
	}
	baselineSummary := summaries[oracle.SideBaseline]
	productionSummary := summaries[oracle.SideProduction]
	if baselineSummary.EqualitySHA256 != productionSummary.EqualitySHA256 {
		t.Fatalf(
			"pair semantic digest differs: baseline=%s production=%s",
			baselineSummary.EqualitySHA256,
			productionSummary.EqualitySHA256,
		)
	}
	if len(baselineSummary.Metrics) != 19 || len(productionSummary.Metrics) != 19 {
		t.Fatalf(
			"pair metric counts differ: baseline=%d production=%d",
			len(baselineSummary.Metrics),
			len(productionSummary.Metrics),
		)
	}
}
