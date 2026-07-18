package runner

import "io"

type captureResult struct {
	payload   []byte
	total     int64
	truncated bool
	err       error
}

func captureStderr(reader io.ReadCloser, limit int, done chan<- captureResult) {
	defer reader.Close()
	buffer := make([]byte, readBufferBytes)
	result := captureResult{payload: make([]byte, 0, limit)}
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			result.total += int64(count)
			remaining := limit - len(result.payload)
			if remaining > 0 {
				result.payload = append(result.payload, buffer[:min(count, remaining)]...)
			}
			result.truncated = result.total > int64(limit)
		}
		if err != nil {
			if err != io.EOF {
				result.err = err
			}
			done <- result
			return
		}
	}
}
