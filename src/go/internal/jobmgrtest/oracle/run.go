package oracle

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/observation"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/reducer"
)

type Side string

const (
	SideBaseline   Side = "baseline"
	SideProduction Side = "production"
)

type Metric string

const (
	MetricMixedP50      Metric = "mixed-p50"
	MetricMixedP95      Metric = "mixed-p95"
	MetricMixedP99      Metric = "mixed-p99"
	MetricJobManagerP50 Metric = "J-p50"
	MetricJobManagerP95 Metric = "J-p95"
	MetricJobManagerP99 Metric = "J-p99"
	MetricFunctionP50   Metric = "F-p50"
	MetricFunctionP95   Metric = "F-p95"
	MetricFunctionP99   Metric = "F-p99"
	MetricCancelP50     Metric = "C-p50"
	MetricCancelP95     Metric = "C-p95"
	MetricCancelP99     Metric = "C-p99"
	MetricDeadlineP50   Metric = "D-p50"
	MetricDeadlineP95   Metric = "D-p95"
	MetricDeadlineP99   Metric = "D-p99"
	MetricCapacityP50   Metric = "K-p50"
	MetricCapacityP95   Metric = "K-p95"
	MetricCapacityP99   Metric = "K-p99"
	MetricComparativeNS Metric = "comparative-ns-per-op"
)

var metrics = [...]Metric{
	MetricMixedP50,
	MetricMixedP95,
	MetricMixedP99,
	MetricJobManagerP50,
	MetricJobManagerP95,
	MetricJobManagerP99,
	MetricFunctionP50,
	MetricFunctionP95,
	MetricFunctionP99,
	MetricCancelP50,
	MetricCancelP95,
	MetricCancelP99,
	MetricDeadlineP50,
	MetricDeadlineP95,
	MetricDeadlineP99,
	MetricCapacityP50,
	MetricCapacityP95,
	MetricCapacityP99,
	MetricComparativeNS,
}

var latencyMetrics = map[contract.PerformanceClass][3]Metric{
	0:                        {MetricMixedP50, MetricMixedP95, MetricMixedP99},
	contract.ClassJobManager: {MetricJobManagerP50, MetricJobManagerP95, MetricJobManagerP99},
	contract.ClassFunction:   {MetricFunctionP50, MetricFunctionP95, MetricFunctionP99},
	contract.ClassCancel:     {MetricCancelP50, MetricCancelP95, MetricCancelP99},
	contract.ClassDeadline:   {MetricDeadlineP50, MetricDeadlineP95, MetricDeadlineP99},
	contract.ClassCapacity:   {MetricCapacityP50, MetricCapacityP95, MetricCapacityP99},
}

type RunObservation struct {
	WorkloadID        string
	Population        int
	PairIndex         int
	Side              Side
	RanFirst          bool
	PairNonce         [16]byte
	EnvironmentSHA256 string
	WallLowerUnix     int64
	WallUpperUnix     int64
	Operations        []reducer.CompletedOperation
}

type RunSummary struct {
	WorkloadID        string
	Population        int
	PairIndex         int
	Side              Side
	RanFirst          bool
	PairNonceSHA256   string
	EnvironmentSHA256 string
	TranscriptSHA256  string
	EqualitySHA256    string
	Metrics           map[Metric]float64
}

func Metrics() []Metric {
	return append([]Metric(nil), metrics[:]...)
}

func AnalyzeRun(run RunObservation) (RunSummary, error) {
	workload, err := findWorkload(run.WorkloadID)
	if err != nil {
		return RunSummary{}, err
	}
	if !validPopulation(run.Population) {
		return RunSummary{}, fmt.Errorf("evaluator oracle: invalid population %d", run.Population)
	}
	if err := validateRunOrder(run.Side, run.PairIndex, run.RanFirst); err != nil {
		return RunSummary{}, err
	}
	if !validDigest(run.EnvironmentSHA256) {
		return RunSummary{}, errors.New("evaluator oracle: invalid environment digest")
	}
	if run.WallLowerUnix > run.WallUpperUnix {
		return RunSummary{}, errors.New("evaluator oracle: inverted run clock bounds")
	}
	if len(run.Operations) != contract.PerformanceOperations {
		return RunSummary{}, fmt.Errorf("evaluator oracle: got %d operations, want %d", len(run.Operations), contract.PerformanceOperations)
	}

	latencies := make(map[contract.PerformanceClass][]int64, len(latencyMetrics))
	for class := range latencyMetrics {
		latencies[class] = make([]int64, 0, contract.PerformanceOperations)
	}
	transcript := sha256.New()
	equality := sha256.New()
	minimumT0 := int64(math.MaxInt64)
	maximumT1 := int64(math.MinInt64)

	for sequence, operation := range run.Operations {
		if operation.Offer.Sequence != sequence {
			return RunSummary{}, fmt.Errorf("evaluator oracle: operation %d has sequence %d", sequence, operation.Offer.Sequence)
		}
		class, key, err := workload.ClassAndKey(sequence)
		if err != nil {
			return RunSummary{}, err
		}
		if err := validateOperation(run, operation, class, key); err != nil {
			return RunSummary{}, fmt.Errorf("evaluator oracle: operation %d: %w", sequence, err)
		}
		latency := max(int64(0), operation.Result.ReadReturnMonoNS-operation.Offer.OfferedMonoNS)
		if _, exists := latencyMetrics[class]; !exists {
			return RunSummary{}, fmt.Errorf("evaluator oracle: unknown class %q", class)
		}
		latencies[0] = append(latencies[0], latency)
		latencies[class] = append(latencies[class], latency)
		minimumT0 = min(minimumT0, operation.Offer.OfferedMonoNS)
		maximumT1 = max(maximumT1, operation.Result.ReadReturnMonoNS)
		if err := appendOperationDigest(transcript, operation, class, key, run.WallLowerUnix, run.WallUpperUnix, true); err != nil {
			return RunSummary{}, fmt.Errorf("evaluator oracle: operation %d transcript: %w", sequence, err)
		}
		if err := appendOperationDigest(equality, operation, class, key, run.WallLowerUnix, run.WallUpperUnix, false); err != nil {
			return RunSummary{}, fmt.Errorf("evaluator oracle: operation %d equality: %w", sequence, err)
		}
	}

	values := make(map[Metric]float64, len(metrics))
	for class, classMetrics := range latencyMetrics {
		percentiles, err := nearestRanks(latencies[class], 50, 95, 99)
		if err != nil {
			return RunSummary{}, fmt.Errorf("evaluator oracle: class %q: %w", class, err)
		}
		for index, metric := range classMetrics {
			values[metric] = float64(percentiles[index])
		}
	}
	values[MetricComparativeNS] = float64(maximumT1-minimumT0) / contract.PerformanceOperations
	if values[MetricComparativeNS] <= 0 || math.IsInf(values[MetricComparativeNS], 0) || math.IsNaN(values[MetricComparativeNS]) {
		return RunSummary{}, errors.New("evaluator oracle: invalid comparative ns/op")
	}

	nonceDigest := sha256.Sum256(run.PairNonce[:])
	return RunSummary{
		WorkloadID: run.WorkloadID, Population: run.Population, PairIndex: run.PairIndex,
		Side: run.Side, RanFirst: run.RanFirst, PairNonceSHA256: hex.EncodeToString(nonceDigest[:]),
		EnvironmentSHA256: run.EnvironmentSHA256, TranscriptSHA256: hex.EncodeToString(transcript.Sum(nil)),
		EqualitySHA256: hex.EncodeToString(equality.Sum(nil)), Metrics: values,
	}, nil
}

func validateOperation(run RunObservation, operation reducer.CompletedOperation, class contract.PerformanceClass, key string) error {
	offer := operation.Offer
	result := operation.Result
	if offer.Class != string([]byte{byte(class)}) || offer.Key != key {
		return fmt.Errorf("schedule is %s/%s, want %c/%s", offer.Class, offer.Key, class, key)
	}
	wantUID := contract.PerformanceUID(run.PairNonce, uint32(offer.Sequence))
	if offer.UID != wantUID || result.UID != wantUID {
		return errors.New("UID differs from pair schedule")
	}
	request, err := contract.PerformanceRequest(class, wantUID, key)
	if err != nil {
		return err
	}
	followup, err := contract.PerformanceFollowup(class, wantUID)
	if err != nil {
		return err
	}
	usefulWork, err := contract.UsefulWorkSHA256(class)
	if err != nil {
		return err
	}
	if offer.RequestSHA256 != digest(request) || offer.FollowupSHA256 != digest(followup) || offer.UsefulWorkSHA256 != usefulWork {
		return errors.New("request, follow-up, or useful-work digest differs")
	}
	if offer.OfferedMonoNS < 0 || result.ReadReturnMonoNS < 0 {
		return errors.New("invalid observation timestamps")
	}
	wantResult, err := contract.ExpectedResult(class)
	if err != nil {
		return err
	}
	if result.Status != wantResult.Status || result.ContentType != wantResult.ContentType ||
		len(result.Payload) != len(wantResult.Payload) || digest(result.Payload) != wantResult.PayloadSHA256 ||
		len(result.Deferred) != len(wantResult.Deferred) || digest(result.Deferred) != wantResult.DeferredSHA256 {
		return errors.New("Function result differs from class contract")
	}
	if result.RawSHA256 != digest(result.Raw) {
		return errors.New("raw Function result digest differs")
	}
	normalized, err := result.NormalizeExpiry(run.WallLowerUnix, run.WallUpperUnix)
	if err != nil {
		return err
	}
	wantNormalized := []byte(fmt.Sprintf("FUNCTION_RESULT_BEGIN %s %d %s @EXPIRY@\n", wantUID, wantResult.Status, wantResult.ContentType))
	wantNormalized = append(wantNormalized, wantResult.Deferred...)
	wantNormalized = append(wantNormalized, []byte("FUNCTION_RESULT_END\n\n")...)
	if !bytes.Equal(normalized, wantNormalized) {
		return errors.New("normalized raw Function result differs")
	}
	return validateOperationEvents(class, offer, result, operation.Events)
}

func validateOperationEvents(class contract.PerformanceClass, offer observation.OfferedRequest, result observation.FunctionResult, events []observation.PassiveEvent) error {
	var handlerEntered, deadlineObserved int
	var handlerEnteredAt int64
	wantRoute := "perf:work-" + offer.Key
	for _, event := range events {
		if err := event.Message.Validate(); err != nil {
			return fmt.Errorf("invalid passive event: %w", err)
		}
		if event.ObservedMonotonicNS < 0 {
			return errors.New("passive event has a negative timestamp")
		}
		if event.Message.Token != offer.UID {
			return errors.New("passive event token differs from offered request")
		}
		if event.Message.RouteKey != wantRoute {
			return fmt.Errorf("passive event route %q differs from %q", event.Message.RouteKey, wantRoute)
		}
		if len(event.Message.Payload) != 0 {
			return errors.New("passive event payload is not empty")
		}
		switch event.Message.Kind {
		case observation.EventHandlerEntered:
			handlerEntered++
			if handlerEntered == 1 {
				handlerEnteredAt = event.ObservedMonotonicNS
			}
		case observation.EventDeadlineObserved:
			deadlineObserved++
		default:
			return fmt.Errorf("unexpected passive event kind %q", event.Message.Kind)
		}
	}
	wantHandlerEntered, wantDeadlineObserved := 0, 0
	switch class {
	case contract.ClassJobManager, contract.ClassFunction, contract.ClassCapacity:
	case contract.ClassCancel:
		wantHandlerEntered = 1
	case contract.ClassDeadline:
		wantDeadlineObserved = 1
	default:
		return fmt.Errorf("unknown performance class %q", class)
	}
	if handlerEntered != wantHandlerEntered {
		return fmt.Errorf("class %c has %d handler-entered events, want %d", class, handlerEntered, wantHandlerEntered)
	}
	if deadlineObserved != wantDeadlineObserved {
		return fmt.Errorf("class %c has %d deadline-observed events, want %d", class, deadlineObserved, wantDeadlineObserved)
	}
	if class == contract.ClassCancel {
		if offer.FollowupMonoNS < handlerEnteredAt {
			return errors.New("cancel follow-up was not written after handler-entered")
		}
	} else if offer.FollowupMonoNS != 0 {
		return errors.New("non-cancel operation has a follow-up timestamp")
	}
	return nil
}

func appendOperationDigest(target hash.Hash, operation reducer.CompletedOperation, class contract.PerformanceClass, key string, lower, upper int64, includeTiming bool) error {
	normalized, err := operation.Result.NormalizeExpiry(lower, upper)
	if err != nil {
		return err
	}
	writeUint64(target, uint64(operation.Offer.Sequence))
	writeString(target, string([]byte{byte(class)}))
	writeString(target, key)
	writeString(target, operation.Offer.UID)
	writeString(target, operation.Offer.RequestSHA256)
	writeString(target, operation.Offer.FollowupSHA256)
	writeString(target, operation.Offer.UsefulWorkSHA256)
	if includeTiming {
		writeUint64(target, uint64(operation.Offer.OfferedMonoNS))
		writeUint64(target, uint64(operation.Offer.FollowupMonoNS))
		writeUint64(target, uint64(operation.Result.ReadReturnMonoNS))
	}
	writeUint64(target, uint64(operation.Result.Status))
	writeString(target, operation.Result.ContentType)
	writeUint64(target, uint64(len(operation.Result.Payload)))
	writeString(target, digest(operation.Result.Payload))
	writeUint64(target, uint64(len(operation.Result.Deferred)))
	writeString(target, digest(operation.Result.Deferred))
	writeString(target, digest(normalized))
	writeUint64(target, uint64(len(operation.Events)))
	for _, event := range operation.Events {
		writeString(target, string(event.Message.Kind))
		writeString(target, event.Message.Token)
		writeString(target, event.Message.RouteKey)
		writeUint64(target, uint64(len(event.Message.Payload)))
		writeString(target, digest(event.Message.Payload))
		if includeTiming {
			writeUint64(target, event.Message.ProcessGeneration)
			writeUint64(target, event.Message.EventSeq)
			writeUint64(target, uint64(event.ObservedMonotonicNS))
		}
	}
	return nil
}

func findWorkload(id string) (contract.PerformanceWorkload, error) {
	for _, workload := range contract.PerformanceWorkloads() {
		if workload.ID == id {
			return workload, nil
		}
	}
	return contract.PerformanceWorkload{}, fmt.Errorf("evaluator oracle: unknown workload %q", id)
}

func validateRunOrder(side Side, pairIndex int, ranFirst bool) error {
	if side != SideBaseline && side != SideProduction {
		return fmt.Errorf("evaluator oracle: invalid side %q", side)
	}
	baselineFirst, err := contract.BaselineRunsFirst(pairIndex)
	if err != nil {
		return fmt.Errorf("evaluator oracle: %w", err)
	}
	wantFirst := baselineFirst == (side == SideBaseline)
	if ranFirst != wantFirst {
		return fmt.Errorf("evaluator oracle: pair %d side %s first=%v, want %v", pairIndex, side, ranFirst, wantFirst)
	}
	return nil
}

func validPopulation(population int) bool {
	return population == 1 || population == 32 || population == 256
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && value == hex.EncodeToString(decoded)
}

func digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func writeString(target hash.Hash, value string) {
	writeUint64(target, uint64(len(value)))
	_, _ = target.Write([]byte(value))
}

func writeUint64(target hash.Hash, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = target.Write(encoded[:])
}
