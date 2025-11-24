package zabbixpreproc

import (
	"testing"
	"time"
)

// TestPerItemStateIsolation tests that different items have isolated state
func TestPerItemStateIsolation(t *testing.T) {
	p := NewPreprocessor("test-shard")

	// Delta value test - two items should have independent state
	step := Step{Type: StepTypeDeltaValue, Params: ""}

	// Item 1: Send values 10, 20, 30
	value1_1 := Value{Data: "10", Type: ValueTypeFloat, Timestamp: time.Now()}
	value1_2 := Value{Data: "20", Type: ValueTypeFloat, Timestamp: time.Now()}
	value1_3 := Value{Data: "30", Type: ValueTypeFloat, Timestamp: time.Now()}

	// Item 2: Send values 100, 200, 300
	value2_1 := Value{Data: "100", Type: ValueTypeFloat, Timestamp: time.Now()}
	value2_2 := Value{Data: "200", Type: ValueTypeFloat, Timestamp: time.Now()}
	value2_3 := Value{Data: "300", Type: ValueTypeFloat, Timestamp: time.Now()}

	// First calls should return 0 (no previous value to delta from)
	result1_1, err1_1 := p.Execute("item1", value1_1, step)
	if err1_1 != nil {
		t.Fatalf("Unexpected error on first call for item1: %v", err1_1)
	}
	if len(result1_1.Metrics) == 0 || result1_1.Metrics[0].Value != "0" {
		t.Errorf("Expected 0 on first call for item1, got %v", result1_1.Metrics)
	}

	result2_1, err2_1 := p.Execute("item2", value2_1, step)
	if err2_1 != nil {
		t.Fatalf("Unexpected error on first call for item2: %v", err2_1)
	}
	if len(result2_1.Metrics) == 0 || result2_1.Metrics[0].Value != "0" {
		t.Errorf("Expected 0 on first call for item2, got %v", result2_1.Metrics)
	}

	// Second calls should succeed with delta
	result1_2, err1_2 := p.Execute("item1", value1_2, step)
	if err1_2 != nil {
		t.Fatalf("Unexpected error for item1 second call: %v", err1_2)
	}
	if len(result1_2.Metrics) == 0 || result1_2.Metrics[0].Value != "10" {
		t.Errorf("Expected delta of 10 for item1, got %v", result1_2.Metrics)
	}

	result2_2, err2_2 := p.Execute("item2", value2_2, step)
	if err2_2 != nil {
		t.Fatalf("Unexpected error for item2 second call: %v", err2_2)
	}
	if len(result2_2.Metrics) == 0 || result2_2.Metrics[0].Value != "100" {
		t.Errorf("Expected delta of 100 for item2, got %v", result2_2.Metrics)
	}

	// Third calls should also succeed with correct deltas
	result1_3, err1_3 := p.Execute("item1", value1_3, step)
	if err1_3 != nil {
		t.Fatalf("Unexpected error for item1 third call: %v", err1_3)
	}
	if len(result1_3.Metrics) == 0 || result1_3.Metrics[0].Value != "10" {
		t.Errorf("Expected delta of 10 for item1, got %v", result1_3.Metrics)
	}

	result2_3, err2_3 := p.Execute("item2", value2_3, step)
	if err2_3 != nil {
		t.Fatalf("Unexpected error for item2 third call: %v", err2_3)
	}
	if len(result2_3.Metrics) == 0 || result2_3.Metrics[0].Value != "100" {
		t.Errorf("Expected delta of 100 for item2, got %v", result2_3.Metrics)
	}
}

// TestPerItemStateIsolation_Throttle tests throttle state isolation
func TestPerItemStateIsolation_Throttle(t *testing.T) {
	p := NewPreprocessor("test-shard")
	step := Step{Type: StepTypeThrottleValue, Params: ""}

	// Item 1 sends "A", "A", "B"
	result1_1, _ := p.Execute("item1", Value{Data: "A", Type: ValueTypeStr, Timestamp: time.Now()}, step)
	result1_2, _ := p.Execute("item1", Value{Data: "A", Type: ValueTypeStr, Timestamp: time.Now()}, step)
	result1_3, _ := p.Execute("item1", Value{Data: "B", Type: ValueTypeStr, Timestamp: time.Now()}, step)

	// Item 2 sends "X", "X", "Y"
	result2_1, _ := p.Execute("item2", Value{Data: "X", Type: ValueTypeStr, Timestamp: time.Now()}, step)
	result2_2, _ := p.Execute("item2", Value{Data: "X", Type: ValueTypeStr, Timestamp: time.Now()}, step)
	result2_3, _ := p.Execute("item2", Value{Data: "Y", Type: ValueTypeStr, Timestamp: time.Now()}, step)

	// Item 1: First value should pass, second should be throttled, third should pass
	if len(result1_1.Metrics) == 0 || result1_1.Metrics[0].Value != "A" {
		t.Errorf("Item1 first call: expected 'A', got %v", result1_1.Metrics)
	}
	if len(result1_2.Metrics) == 0 || result1_2.Metrics[0].Value != "" {
		t.Errorf("Item1 second call: expected empty (throttled), got %v", result1_2.Metrics)
	}
	if len(result1_3.Metrics) == 0 || result1_3.Metrics[0].Value != "B" {
		t.Errorf("Item1 third call: expected 'B', got %v", result1_3.Metrics)
	}

	// Item 2: Same pattern (independent of item1)
	if len(result2_1.Metrics) == 0 || result2_1.Metrics[0].Value != "X" {
		t.Errorf("Item2 first call: expected 'X', got %v", result2_1.Metrics)
	}
	if len(result2_2.Metrics) == 0 || result2_2.Metrics[0].Value != "" {
		t.Errorf("Item2 second call: expected empty (throttled), got %v", result2_2.Metrics)
	}
	if len(result2_3.Metrics) == 0 || result2_3.Metrics[0].Value != "Y" {
		t.Errorf("Item2 third call: expected 'Y', got %v", result2_3.Metrics)
	}
}

// TestShardIDInStateKey tests that shardID is part of state key
func TestShardIDInStateKey(t *testing.T) {
	// Two preprocessors with different shards
	p1 := NewPreprocessor("shard1")
	p2 := NewPreprocessor("shard2")

	step := Step{Type: StepTypeDeltaValue, Params: ""}

	// Both use same itemID but different shards
	itemID := "same-item"

	// Shard 1: Send 10, 20
	_, _ = p1.Execute(itemID, Value{Data: "10", Type: ValueTypeFloat, Timestamp: time.Now()}, step)
	result1, err1 := p1.Execute(itemID, Value{Data: "20", Type: ValueTypeFloat, Timestamp: time.Now()}, step)

	// Shard 2: Send 100, 200
	_, _ = p2.Execute(itemID, Value{Data: "100", Type: ValueTypeFloat, Timestamp: time.Now()}, step)
	result2, err2 := p2.Execute(itemID, Value{Data: "200", Type: ValueTypeFloat, Timestamp: time.Now()}, step)

	// Both should succeed with correct deltas (proves state isolation)
	if err1 != nil {
		t.Fatalf("Shard1 error: %v", err1)
	}
	if len(result1.Metrics) == 0 || result1.Metrics[0].Value != "10" {
		t.Errorf("Shard1: expected delta of 10, got %v", result1.Metrics)
	}

	if err2 != nil {
		t.Fatalf("Shard2 error: %v", err2)
	}
	if len(result2.Metrics) == 0 || result2.Metrics[0].Value != "100" {
		t.Errorf("Shard2: expected delta of 100, got %v", result2.Metrics)
	}
}

// TestStateKeyFormat tests that state keys follow shardID:itemID:operation format
func TestStateKeyFormat(t *testing.T) {
	p := NewPreprocessor("my-shard")
	step := Step{Type: StepTypeDeltaValue, Params: ""}

	// Execute to populate state
	_, _ = p.Execute("my-item", Value{Data: "10", Type: ValueTypeFloat, Timestamp: time.Now()}, step)

	// Check that state key exists with correct format
	expectedKey := "my-shard:my-item:delta_value"
	if _, exists := p.state[expectedKey]; !exists {
		t.Errorf("Expected state key '%s' not found. Existing keys: %v", expectedKey, getStateKeys(p))
	}
}

func TestClearStateRemovesItemEntries(t *testing.T) {
	p := NewPreprocessor("clear-shard")
	step := Step{Type: StepTypeDeltaValue, Params: ""}

	// Populate state for two different items
	_, _ = p.Execute("item1", Value{Data: "10", Type: ValueTypeFloat, Timestamp: time.Now()}, step)
	_, _ = p.Execute("item1", Value{Data: "20", Type: ValueTypeFloat, Timestamp: time.Now()}, step)
	_, _ = p.Execute("item2", Value{Data: "100", Type: ValueTypeFloat, Timestamp: time.Now()}, step)
	_, _ = p.Execute("item2", Value{Data: "200", Type: ValueTypeFloat, Timestamp: time.Now()}, step)

	if len(getStateKeys(p)) != 2 {
		t.Fatalf("expected state for two items, got keys %v", getStateKeys(p))
	}

	// Clear item1 state and verify only item2 remains
	p.ClearState("item1")
	keys := getStateKeys(p)
	if len(keys) != 1 || keys[0] != "clear-shard:item2:delta_value" {
		t.Fatalf("expected only item2 state to remain, got %v", keys)
	}

	// Ensure clearing non-existing item is no-op
	p.ClearState("missing")
	keys = getStateKeys(p)
	if len(keys) != 1 || keys[0] != "clear-shard:item2:delta_value" {
		t.Fatalf("clear of missing item altered state: %v", keys)
	}
}

// Helper to get state keys for debugging
func getStateKeys(p *Preprocessor) []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	keys := make([]string, 0, len(p.state))
	for k := range p.state {
		keys = append(keys, k)
	}
	return keys
}
