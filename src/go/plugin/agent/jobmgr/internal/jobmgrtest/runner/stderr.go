package runner

import "io"

type captureResult struct {
	payload   []byte
	truncated bool
	err       error
}

func captureStderr(reader io.ReadCloser, done chan<- captureResult) {
	defer reader.Close()
	buffer := make([]byte, readBufferBytes)
	result := captureResult{payload: make([]byte, 0, stderrLimit)}
	var total int64
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			total += int64(count)
			remaining := stderrLimit - len(result.payload)
			if remaining > 0 {
				result.payload = append(result.payload, buffer[:min(count, remaining)]...)
			}
			result.truncated = total > int64(stderrLimit)
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
