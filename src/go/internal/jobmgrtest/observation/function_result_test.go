package observation

import (
	"bytes"
	"strings"
	"testing"
)

func TestFunctionResultDecoderFragmentationNoiseAndSharedTimestamp(t *testing.T) {
	decoder, err := NewFunctionResultDecoder(4_096)
	if err != nil {
		t.Fatal(err)
	}
	stream := "CONFIG x create accepted\n" +
		"FUNCTION_RESULT_BEGIN u1 200 application/json 100\n{\"status\":200}\nFUNCTION_RESULT_END\n\n" +
		"CHART x\n" +
		"FUNCTION_RESULT_BEGIN u2 499 application/json 101\nFUNCTION_RESULT_END\n\n"
	var results []FunctionResult
	var skipped []byte
	for offset, width := 0, 1; offset < len(stream); width++ {
		end := min(len(stream), offset+width)
		got, noise, err := decoder.Feed([]byte(stream[offset:end]), int64(end))
		if err != nil {
			t.Fatal(err)
		}
		results = append(results, got...)
		skipped = append(skipped, noise...)
		offset = end
	}
	remaining, err := decoder.Finish()
	if err != nil {
		t.Fatal(err)
	}
	skipped = append(skipped, remaining...)
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	if results[0].UID != "u1" || string(results[0].Payload) != `{"status":200}` || len(results[0].Deferred) != 15 {
		t.Fatalf("first result differs: %#v", results[0])
	}
	if results[1].UID != "u2" || results[1].Payload != nil || results[1].Status != 499 {
		t.Fatalf("second result differs: %#v", results[1])
	}
	if string(skipped) != "CONFIG x create accepted\nCHART x\n" {
		t.Fatalf("noise differs: %q", skipped)
	}
}

func TestFunctionResultDecoderOneReadSharesCompletionTimestamp(t *testing.T) {
	decoder, err := NewFunctionResultDecoder(4_096)
	if err != nil {
		t.Fatal(err)
	}
	stream := "FUNCTION_RESULT_BEGIN u1 200 application/json 100\n{}\nFUNCTION_RESULT_END\n\n" +
		"FUNCTION_RESULT_BEGIN u2 200 application/json 100\n{}\nFUNCTION_RESULT_END\n\n"
	results, skipped, err := decoder.Feed([]byte(stream), 77)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 || len(results) != 2 || results[0].ReadReturnMonoNS != 77 || results[1].ReadReturnMonoNS != 77 {
		t.Fatalf("one-read completion differs: results=%#v skipped=%q", results, skipped)
	}
}

func TestFunctionResultExpiryNormalizationChangesOnlyExpiry(t *testing.T) {
	raw := []byte("FUNCTION_RESULT_BEGIN u1 200 application/json 101\n{\"status\":200}\nFUNCTION_RESULT_END\n\n")
	result, err := parseFunctionResult(raw, 1)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := result.NormalizeExpiry(100, 102)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(raw, []byte(" 101\n"), []byte(" @EXPIRY@\n"), 1)
	if !bytes.Equal(normalized, want) {
		t.Fatalf("normalized frame differs:\n got %q\nwant %q", normalized, want)
	}
	if _, err := result.NormalizeExpiry(102, 103); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("out-of-range expiry accepted: %v", err)
	}
}

func TestFunctionResultDecoderRejectsMalformedAndOversizedFrames(t *testing.T) {
	decoder, err := NewFunctionResultDecoder(96)
	if err != nil {
		t.Fatal(err)
	}
	oversized := []byte("FUNCTION_RESULT_BEGIN u1 200 application/json 100\n" + strings.Repeat("x", 96))
	if _, _, err := decoder.Feed(oversized, 1); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized frame accepted: %v", err)
	}
	decoder, err = NewFunctionResultDecoder(4_096)
	if err != nil {
		t.Fatal(err)
	}
	malformed := []byte("FUNCTION_RESULT_BEGIN u1 nope application/json 100\n{}\nFUNCTION_RESULT_END\n\n")
	if _, _, err := decoder.Feed(malformed, 1); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("malformed status accepted: %v", err)
	}
}
