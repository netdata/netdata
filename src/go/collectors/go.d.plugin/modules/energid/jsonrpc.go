// SPDX-License-Identifier: GPL-3.0-or-later

package energid

import (
	"encoding/json"
	"fmt"
)

// https://www.jsonrpc.org/specification#request_object
type (
	rpcRequest struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		ID      int    `json:"id"`
	}
	rpcRequests []rpcRequest
)

// http://www.jsonrpc.org/specification#response_object
type (
	rpcResponse struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
		Error   *rpcError       `json:"error"`
		ID      int             `json:"id"`
	}
	rpcResponses []rpcResponse
)

func (rs rpcResponses) getByID(id int) *rpcResponse {
	for _, r := range rs {
		if r.ID == id {
			return &r
		}
	}
	return nil
}

// http://www.jsonrpc.org/specification#error_object
type rpcError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

func (e rpcError) String() string {
	return fmt.Sprintf("%s (code %d)", e.Message, e.Code)
}
