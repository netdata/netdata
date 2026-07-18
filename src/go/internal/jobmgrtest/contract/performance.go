package contract

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	PerformanceFixtureCount = 256
	PerformanceOperations   = 10_240
	PerformancePairCount    = 15
	PerformancePairSeed     = 4_605
)

type PerformanceClass byte

const (
	ClassJobManager PerformanceClass = 'J'
	ClassFunction   PerformanceClass = 'F'
	ClassCancel     PerformanceClass = 'C'
	ClassDeadline   PerformanceClass = 'D'
	ClassCapacity   PerformanceClass = 'K'
)

type PerformanceFixture struct {
	Ordinal            uint16
	KeyToken           string
	FunctionID         string
	PublicFunctionName string
	ConfigName         string
	PublicConfigID     string
}

func Fixture(ordinal int) (PerformanceFixture, error) {
	if ordinal < 0 || ordinal >= PerformanceFixtureCount {
		return PerformanceFixture{}, fmt.Errorf("performance fixture ordinal %d outside 0..255", ordinal)
	}
	key := fmt.Sprintf("%03d", ordinal)
	return PerformanceFixture{
		Ordinal: uint16(ordinal), KeyToken: key,
		FunctionID: "work-" + key, PublicFunctionName: "perf:work-" + key,
		ConfigName: "job-" + key, PublicConfigID: "poc:collector:perf:job-" + key,
	}, nil
}

type PerformanceResult struct {
	Status         int
	ContentType    string
	Payload        []byte
	PayloadSHA256  string
	Deferred       []byte
	DeferredSHA256 string
}

func ExpectedResult(class PerformanceClass) (PerformanceResult, error) {
	var status int
	var payload []byte
	switch class {
	case ClassJobManager:
		status = 200
		payload = []byte(`{"option_str":"work","option_int":1}`)
	case ClassFunction:
		status = 200
		payload = []byte(`{"status":200}`)
	case ClassCancel:
		status = 499
		payload = []byte(`{"errorMessage":"Request cancelled.","status":499}`)
	case ClassDeadline:
		status = 504
		payload = []byte(`{"errorMessage":"Deadline exceeded.","status":504}`)
	case ClassCapacity:
		status = 200
		payload = []byte(`{"pad":"` + strings.Repeat("A", 4_072) + `","status":200}`)
	default:
		return PerformanceResult{}, fmt.Errorf("unknown performance class %q", class)
	}
	deferred := append(append([]byte(nil), payload...), '\n')
	return PerformanceResult{
		Status: status, ContentType: "application/json", Payload: payload, PayloadSHA256: sha256Hex(payload),
		Deferred: deferred, DeferredSHA256: sha256Hex(deferred),
	}, nil
}

func PerformanceUID(pairNonce [16]byte, sequence uint32) string {
	return scheduledUID('u', "jmperf-uid-v1\x00", pairNonce[:], sequence)
}

func SetupUID(pairNonce [16]byte, key uint16) string {
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], key)
	return domainUID('s', "jmperf-setup-uid-v1\x00", pairNonce[:], encoded[:])
}

func PerformanceRequest(class PerformanceClass, uid, key string) ([]byte, error) {
	if !validUID(uid) {
		return nil, errors.New("invalid performance UID")
	}
	if !validKey(key) {
		return nil, errors.New("invalid performance key")
	}
	var call string
	timeout := 30
	switch class {
	case ClassJobManager:
		call = "config poc:collector:perf:job-" + key + " get"
	case ClassFunction, ClassCancel, ClassCapacity:
		call = fmt.Sprintf("perf:work-%s mode:%c token:%s", key, class, uid)
	case ClassDeadline:
		timeout = 0
		call = fmt.Sprintf("perf:work-%s mode:%c token:%s", key, class, uid)
	default:
		return nil, fmt.Errorf("unknown performance class %q", class)
	}
	return []byte(fmt.Sprintf("FUNCTION %s %d %q 0xFFFF %q\n", uid, timeout, call, "method=api,role=test")), nil
}

func PerformanceFollowup(class PerformanceClass, uid string) ([]byte, error) {
	if !validUID(uid) {
		return nil, errors.New("invalid performance UID")
	}
	if class != ClassCancel {
		return nil, nil
	}
	return []byte("FUNCTION_CANCEL " + uid + "\n"), nil
}

type PerformanceWorkload struct {
	ID          string
	Pattern     []PerformanceClass
	Repetitions int
	ClassCounts map[PerformanceClass]int
	PerKey      map[PerformanceClass]int
}

func PerformanceWorkloads() []PerformanceWorkload {
	return []PerformanceWorkload{
		workload("B-WL-001-balanced", "JFJCJDJKJK", 1_024, [5]int{5_120, 1_024, 1_024, 1_024, 2_048}, [5]int{20, 4, 4, 4, 8}),
		workload("B-WL-002-jm-heavy", "JJJJJJJFJJCJJDJJDJKK", 512, [5]int{7_168, 512, 512, 1_024, 1_024}, [5]int{28, 2, 2, 4, 4}),
		workload("B-WL-003-function-heavy", "JFCDK", 2_048, [5]int{2_048, 2_048, 2_048, 2_048, 2_048}, [5]int{8, 8, 8, 8, 8}),
	}
}

func (workload PerformanceWorkload) ClassAndKey(sequence int) (PerformanceClass, string, error) {
	if sequence < 0 || sequence >= PerformanceOperations || len(workload.Pattern) == 0 {
		return 0, "", fmt.Errorf("performance sequence %d outside workload", sequence)
	}
	class := workload.Pattern[sequence%len(workload.Pattern)]
	repetition := sequence / len(workload.Pattern)
	return class, fmt.Sprintf("%03d", repetition%PerformanceFixtureCount), nil
}

func BaselineRunsFirst(pairIndex int) (bool, error) {
	if pairIndex < 0 || pairIndex >= PerformancePairCount {
		return false, fmt.Errorf("performance pair %d outside 0..14", pairIndex)
	}
	return ((PerformancePairSeed + pairIndex) & 1) == 0, nil
}

// UsefulWorkSHA256 pins the executable class contract shared by both sides.
func UsefulWorkSHA256(class PerformanceClass) (string, error) {
	result, err := ExpectedResult(class)
	if err != nil {
		return "", err
	}
	contract := struct {
		Class          string `json:"class"`
		RequestProfile string `json:"request_profile"`
		Followup       string `json:"followup"`
		HandlerWork    string `json:"handler_work"`
		Status         int    `json:"status"`
		ContentType    string `json:"content_type"`
		PayloadLen     int    `json:"payload_len"`
		PayloadSHA256  string `json:"payload_sha256"`
		DeferredLen    int    `json:"deferred_len"`
		DeferredSHA256 string `json:"deferred_sha256"`
	}{
		Class: string([]byte{byte(class)}), RequestProfile: requestProfile(class), Followup: followupProfile(class),
		HandlerWork: handlerWork(class), Status: result.Status, ContentType: result.ContentType,
		PayloadLen: len(result.Payload), PayloadSHA256: result.PayloadSHA256,
		DeferredLen: len(result.Deferred), DeferredSHA256: result.DeferredSHA256,
	}
	payload, err := json.Marshal(contract)
	if err != nil {
		return "", err
	}
	return sha256Hex(payload), nil
}

func workload(id, pattern string, repetitions int, totals, perKey [5]int) PerformanceWorkload {
	classes := []PerformanceClass{ClassJobManager, ClassFunction, ClassCancel, ClassDeadline, ClassCapacity}
	parsed := make([]PerformanceClass, len(pattern))
	for index := range pattern {
		parsed[index] = PerformanceClass(pattern[index])
	}
	result := PerformanceWorkload{ID: id, Pattern: parsed, Repetitions: repetitions, ClassCounts: make(map[PerformanceClass]int, 5), PerKey: make(map[PerformanceClass]int, 5)}
	for index, class := range classes {
		result.ClassCounts[class] = totals[index]
		result.PerKey[class] = perKey[index]
	}
	return result
}

func scheduledUID(prefix byte, domain string, nonce []byte, sequence uint32) string {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], sequence)
	return domainUID(prefix, domain, nonce, encoded[:])
}

func domainUID(prefix byte, domain string, nonce, suffix []byte) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte(domain))
	_, _ = hash.Write(nonce)
	_, _ = hash.Write(suffix)
	sum := hash.Sum(nil)
	return string(prefix) + hex.EncodeToString(sum[:16])
}

func validUID(uid string) bool {
	if len(uid) != 33 || (uid[0] != 'u' && uid[0] != 's') {
		return false
	}
	_, err := hex.DecodeString(uid[1:])
	return err == nil
}

func validKey(key string) bool {
	if len(key) != 3 {
		return false
	}
	for _, char := range key {
		if char < '0' || char > '9' {
			return false
		}
	}
	return key <= "255"
}

func requestProfile(class PerformanceClass) string {
	if class == ClassJobManager {
		return `FUNCTION uid 30 "config poc:collector:perf:job-KKK get" 0xFFFF "method=api,role=test" LF`
	}
	timeout := 30
	if class == ClassDeadline {
		timeout = 0
	}
	return fmt.Sprintf(`FUNCTION uid %d "perf:work-KKK mode:%c token:uid" 0xFFFF "method=api,role=test" LF`, timeout, class)
}

func followupProfile(class PerformanceClass) string {
	if class == ClassCancel {
		return "after-handler-entered:FUNCTION_CANCEL uid LF;does-not-reset-t0"
	}
	return "none"
}

func handlerWork(class PerformanceClass) string {
	switch class {
	case ClassJobManager:
		return "public-jm-get-create-apply-configuration-json"
	case ClassFunction:
		return "return-prebuilt-status-200"
	case ClassCancel:
		return "emit-handler-entered-wait-context-canceled"
	case ClassDeadline:
		return "wait-context-deadline-exceeded-timeout-zero"
	case ClassCapacity:
		return "return-prebuilt-4072-byte-pad-status-200"
	default:
		return "invalid"
	}
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
