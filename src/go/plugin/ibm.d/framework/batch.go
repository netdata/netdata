package framework

// Batch creates batches from a slice of items
func Batch[T any](items []T, size int) <-chan []T {
	ch := make(chan []T)
	
	go func() {
		defer close(ch)
		
		for i := 0; i < len(items); i += size {
			end := i + size
			if end > len(items) {
				end = len(items)
			}
			
			ch <- items[i:end]
		}
	}()
	
	return ch
}

// BatchWithError creates batches and allows error handling
func BatchWithError[T any](items []T, size int, fn func([]T) error) error {
	for batch := range Batch(items, size) {
		if err := fn(batch); err != nil {
			return err
		}
	}
	return nil
}

// ParallelBatch processes batches in parallel with worker pool
func ParallelBatch[T any](items []T, batchSize int, workers int, fn func([]T) error) error {
	type result struct {
		err error
	}
	
	work := make(chan []T, workers)
	results := make(chan result, workers)
	
	// Start workers
	for i := 0; i < workers; i++ {
		go func() {
			for batch := range work {
				results <- result{err: fn(batch)}
			}
		}()
	}
	
	// Send work
	go func() {
		for batch := range Batch(items, batchSize) {
			work <- batch
		}
		close(work)
	}()
	
	// Collect results
	var firstErr error
	batchCount := (len(items) + batchSize - 1) / batchSize
	for i := 0; i < batchCount; i++ {
		res := <-results
		if res.err != nil && firstErr == nil {
			firstErr = res.err
		}
	}
	
	return firstErr
}

// Min returns the minimum of two integers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the maximum of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}