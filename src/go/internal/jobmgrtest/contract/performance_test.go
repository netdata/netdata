package contract

import (
	"bytes"
	"strings"
	"testing"
)

func TestPerformanceFixturesAreExact(t *testing.T) {
	first, err := Fixture(0)
	if err != nil {
		t.Fatal(err)
	}
	last, err := Fixture(255)
	if err != nil {
		t.Fatal(err)
	}
	if first.KeyToken != "000" || first.PublicFunctionName != "perf:work-000" || first.PublicConfigID != "poc:collector:perf:job-000" {
		t.Fatalf("first fixture differs: %#v", first)
	}
	if last.KeyToken != "255" || last.PublicFunctionName != "perf:work-255" || last.PublicConfigID != "poc:collector:perf:job-255" {
		t.Fatalf("last fixture differs: %#v", last)
	}
	if _, err := Fixture(-1); err == nil {
		t.Fatal("negative fixture accepted")
	}
	if _, err := Fixture(PerformanceFixtureCount); err == nil {
		t.Fatal("fixture 256 accepted")
	}
}

func TestPerformanceResultsMatchFrozenBytes(t *testing.T) {
	want := map[PerformanceClass]struct {
		status, payloadLen, deferredLen int
		payloadSHA, deferredSHA         string
	}{
		ClassJobManager: {200, 36, 37, "e7a4111051c8f56e571e3646e8e7f07f97eb3046b5f5bab31b36179546d4ee97", "119c023e185ffa4bbe3fe878d7d7d7b86c6612677cf84d1b6c367a73f8666994"},
		ClassFunction:   {200, 14, 15, "7cd85494eb375cc958155aca095fd0bae01e24f777c4ce4059e2edb82324618c", "50d97e5f27f239267fec2999bbb75a0f1f894549750144785fbf15f9b936168a"},
		ClassCancel:     {499, 50, 51, "db17f19063a6618c0fcfeb9000854b5dd8dd67155236e12d730a8b4e07c63349", "e96048adca5ef3d4cda78ea680b99efe95e23ca947521f9379b828013be82767"},
		ClassDeadline:   {504, 50, 51, "38e04c46f168dd70c79c5db91ba2c32f32df953b2f354bdc4024ca9c818d2f1e", "b63e15d74332da71f3e6fe9d4d4d626895524e3bfcd47abee527d0ef5071d11f"},
		ClassCapacity:   {200, 4_095, 4_096, "244a805591c32383a2123fb449897ea3c31cd587111c4f41898f0db82aeb60ad", "e653105e8b35ed140626c4ee977ec8629c5b6aa581313741b5243641a2863f90"},
	}
	for class, expected := range want {
		result, err := ExpectedResult(class)
		if err != nil {
			t.Fatal(err)
		}
		mismatch := []bool{
			result.Status != expected.status,
			len(result.Payload) != expected.payloadLen,
			len(result.Deferred) != expected.deferredLen,
			result.PayloadSHA256 != expected.payloadSHA,
			result.DeferredSHA256 != expected.deferredSHA,
		}
		if mismatch[0] || mismatch[1] || mismatch[2] || mismatch[3] || mismatch[4] {
			t.Fatalf("class %c result differs: status=%d/%d payload=%d/%d %s/%s deferred=%d/%d %s/%s mismatch=%v", class,
				result.Status, expected.status, len(result.Payload), expected.payloadLen, result.PayloadSHA256, expected.payloadSHA,
				len(result.Deferred), expected.deferredLen, result.DeferredSHA256, expected.deferredSHA, mismatch)
		}
	}
}

func TestPerformanceRequestSemantics(t *testing.T) {
	var nonce [16]byte
	copy(nonce[:], "shared-pair-nonc")
	uid := PerformanceUID(nonce, 7)
	for _, class := range []PerformanceClass{ClassJobManager, ClassFunction, ClassCancel, ClassDeadline, ClassCapacity} {
		request, err := PerformanceRequest(class, uid, "007")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasSuffix(request, []byte("\n")) || !bytes.Contains(request, []byte(`0xFFFF "method=api,role=test"`)) {
			t.Fatalf("class %c request framing differs: %q", class, request)
		}
		if class == ClassDeadline && !bytes.Contains(request, []byte(" 0 ")) {
			t.Fatalf("deadline request is not immediate: %q", request)
		}
		followup, err := PerformanceFollowup(class, uid)
		if err != nil {
			t.Fatal(err)
		}
		if class == ClassCancel && string(followup) != "FUNCTION_CANCEL "+uid+"\n" {
			t.Fatalf("cancel followup differs: %q", followup)
		}
		if class != ClassCancel && followup != nil {
			t.Fatalf("class %c has unexpected followup %q", class, followup)
		}
	}
	if got := PerformanceUID(nonce, 7); got != uid || len(got) != 33 || !strings.HasPrefix(got, "u") {
		t.Fatalf("UID schedule is not deterministic: %q", got)
	}
}

func TestPerformanceWorkloadCountsAndSchedule(t *testing.T) {
	workloads := PerformanceWorkloads()
	wantIDs := []string{"B-WL-001-balanced", "B-WL-002-jm-heavy", "B-WL-003-function-heavy"}
	if len(workloads) != len(wantIDs) {
		t.Fatalf("got %d workloads, want %d", len(workloads), len(wantIDs))
	}
	for index, workload := range workloads {
		if workload.ID != wantIDs[index] {
			t.Fatalf("workload %d ID %q, want %q", index, workload.ID, wantIDs[index])
		}
		if got := len(workload.Pattern) * workload.Repetitions; got != PerformanceOperations {
			t.Fatalf("%s has %d operations", workload.ID, got)
		}
		counts := make(map[PerformanceClass]int, 5)
		perKey := make(map[string]map[PerformanceClass]int, PerformanceFixtureCount)
		for sequence := range PerformanceOperations {
			class, key, err := workload.ClassAndKey(sequence)
			if err != nil {
				t.Fatal(err)
			}
			counts[class]++
			if perKey[key] == nil {
				perKey[key] = make(map[PerformanceClass]int, 5)
			}
			perKey[key][class]++
		}
		if len(perKey) != PerformanceFixtureCount {
			t.Fatalf("%s touched %d keys", workload.ID, len(perKey))
		}
		for class, want := range workload.ClassCounts {
			if counts[class] != want {
				t.Fatalf("%s class %c count %d, want %d", workload.ID, class, counts[class], want)
			}
		}
		for key, keyCounts := range perKey {
			for class, want := range workload.PerKey {
				if keyCounts[class] != want {
					t.Fatalf("%s key %s class %c count %d, want %d", workload.ID, key, class, keyCounts[class], want)
				}
			}
		}
	}
}

func TestPerformancePairOrderAlternates(t *testing.T) {
	for pair := range PerformancePairCount {
		baselineFirst, err := BaselineRunsFirst(pair)
		if err != nil {
			t.Fatal(err)
		}
		if baselineFirst != (pair%2 == 1) {
			t.Fatalf("pair %d current-first=%v", pair, baselineFirst)
		}
	}
}

func TestUsefulWorkDigestsAreClassDistinct(t *testing.T) {
	seen := make(map[string]PerformanceClass, 5)
	for _, class := range []PerformanceClass{ClassJobManager, ClassFunction, ClassCancel, ClassDeadline, ClassCapacity} {
		digest, err := UsefulWorkSHA256(class)
		if err != nil {
			t.Fatal(err)
		}
		if previous, exists := seen[digest]; exists {
			t.Fatalf("classes %c and %c share useful-work digest", previous, class)
		}
		seen[digest] = class
	}
}
