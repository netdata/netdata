package zabbixpreproc

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestConcurrentDeltaValue tests concurrent access to deltaValue operation
func TestConcurrentDeltaValue(t *testing.T) {
	p := NewPreprocessor("test-shard")
	step := Step{
		Type:   StepTypeDeltaValue,
		Params: "",
	}

	var wg sync.WaitGroup
	goroutines := 100
	iterations := 10

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			itemID := fmt.Sprintf("item-%d", id) // Each goroutine uses different itemID
			for j := 0; j < iterations; j++ {
				value := Value{
					Data:      fmt.Sprintf("%d", id*iterations+j),
					Type:      ValueTypeFloat,
					Timestamp: time.Now(),
				}
				_, err := p.Execute(itemID, value, step)
				// First call returns error (no previous value), subsequent calls should succeed
				if err != nil && j > 0 {
					t.Errorf("goroutine %d iteration %d: unexpected error: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentDeltaSpeed tests concurrent access to deltaSpeed operation
func TestConcurrentDeltaSpeed(t *testing.T) {
	p := NewPreprocessor("test-shard")
	step := Step{
		Type:   StepTypeDeltaSpeed,
		Params: "",
	}

	var wg sync.WaitGroup
	goroutines := 100
	iterations := 10

	baseTime := time.Now()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			itemID := fmt.Sprintf("item-%d", id)
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				value := Value{
					Data:      fmt.Sprintf("%d", id*iterations+j),
					Type:      ValueTypeFloat,
					Timestamp: baseTime.Add(time.Duration(id*iterations+j) * time.Second),
				}
				_, err := p.Execute(itemID, value, step)
				// First call returns error (no previous value), subsequent calls should succeed
				if err != nil && j > 0 {
					t.Errorf("goroutine %d iteration %d: unexpected error: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentThrottleValue tests concurrent access to throttleValue operation
func TestConcurrentThrottleValue(t *testing.T) {
	p := NewPreprocessor("test-shard")
	step := Step{
		Type:   StepTypeThrottleValue,
		Params: "",
	}

	var wg sync.WaitGroup
	goroutines := 100
	iterations := 10

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			itemID := fmt.Sprintf("item-%d", id)
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				value := Value{
					Data:      fmt.Sprintf("value-%d-%d", id, j),
					Type:      ValueTypeStr,
					Timestamp: time.Now(),
				}
				_, err := p.Execute(itemID, value, step)
				if err != nil {
					t.Errorf("goroutine %d iteration %d: unexpected error: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentThrottleTimedValue tests concurrent access to throttleTimedValue operation
func TestConcurrentThrottleTimedValue(t *testing.T) {
	p := NewPreprocessor("test-shard")
	step := Step{
		Type:   StepTypeThrottleTimedValue,
		Params: "1s",
	}

	var wg sync.WaitGroup
	goroutines := 100
	iterations := 10

	baseTime := time.Now()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			itemID := fmt.Sprintf("item-%d", id)
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				value := Value{
					Data:      fmt.Sprintf("value-%d-%d", id, j),
					Type:      ValueTypeStr,
					Timestamp: baseTime.Add(time.Duration(id*iterations+j) * time.Millisecond * 100),
				}
				_, err := p.Execute(itemID, value, step)
				if err != nil {
					t.Errorf("goroutine %d iteration %d: unexpected error: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentMixedOperations tests concurrent access to mixed stateful operations
func TestConcurrentMixedOperations(t *testing.T) {
	p := NewPreprocessor("test-shard")

	steps := []Step{
		{Type: StepTypeDeltaValue, Params: ""},
		{Type: StepTypeDeltaSpeed, Params: ""},
		{Type: StepTypeThrottleValue, Params: ""},
		{Type: StepTypeThrottleTimedValue, Params: "1s"},
	}

	var wg sync.WaitGroup
	goroutines := 50
	iterations := 10

	baseTime := time.Now()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			itemID := fmt.Sprintf("item-%d", id)
			defer wg.Done()
			step := steps[id%len(steps)]
			for j := 0; j < iterations; j++ {
				value := Value{
					Data:      fmt.Sprintf("%d", id*iterations+j),
					Type:      ValueTypeFloat,
					Timestamp: baseTime.Add(time.Duration(id*iterations+j) * time.Millisecond * 50),
				}
				_, err := p.Execute(itemID, value, step)
				// Delta operations fail on first call (no previous value)
				if err != nil && (step.Type == StepTypeDeltaValue || step.Type == StepTypeDeltaSpeed) && j > 0 {
					t.Errorf("goroutine %d iteration %d: unexpected error: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentPipelines tests concurrent execution of preprocessing pipelines
func TestConcurrentPipelines(t *testing.T) {
	p := NewPreprocessor("test-shard")

	// Pipeline with multiple stateful operations
	pipeline := []Step{
		{Type: StepTypeTrim, Params: " "},
		{Type: StepTypeDeltaValue, Params: ""},
		{Type: StepTypeThrottleValue, Params: ""},
	}

	var wg sync.WaitGroup
	goroutines := 50
	iterations := 10

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			itemID := fmt.Sprintf("item-%d", id)
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				value := Value{
					Data:      fmt.Sprintf(" %d ", id*iterations+j),
					Type:      ValueTypeFloat,
					Timestamp: time.Now(),
				}
				_, err := p.ExecutePipeline(itemID, value, pipeline)
				// First call will fail at deltaValue (no previous value)
				if err != nil && j > 0 {
					t.Errorf("goroutine %d iteration %d: unexpected error: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentMultiplePreprocessors tests that separate preprocessor instances are isolated
func TestConcurrentMultiplePreprocessors(t *testing.T) {
	preprocessors := make([]*Preprocessor, 10)
	for i := range preprocessors {
		preprocessors[i] = NewPreprocessor(fmt.Sprintf("shard-%d", i))
	}

	step := Step{
		Type:   StepTypeDeltaValue,
		Params: "",
	}

	var wg sync.WaitGroup

	for i, p := range preprocessors {
		wg.Add(1)
		go func(id int, preprocessor *Preprocessor) {
			itemID := fmt.Sprintf("item-%d", id)
			defer wg.Done()
			for j := 0; j < 100; j++ {
				value := Value{
					Data:      fmt.Sprintf("%d", j),
					Type:      ValueTypeFloat,
					Timestamp: time.Now(),
				}
				_, err := preprocessor.Execute(itemID, value, step)
				// First call returns error (no previous value)
				if err != nil && j > 0 {
					t.Errorf("preprocessor %d iteration %d: unexpected error: %v", id, j, err)
				}
			}
		}(i, p)
	}

	wg.Wait()
}
