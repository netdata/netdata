package raw

import "errors"

// DispatchHandler validates/decodes a single service kind request and writes
// the matching response into responseBuf.
type DispatchHandler func(request []byte, responseBuf []byte) (int, error)

var errHandlerFailed = errors.New("dispatch handler failed")
