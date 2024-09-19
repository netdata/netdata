// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

type (
	OsdPoolStats struct {
		PoolName     string       `json:"pool_name"`
		PoolID       int64        `json:"pool_id"`
		Recovery     Recovery     `json:"recovery"`
		RecoveryRate RecoveryRate `json:"recovery_rate"`
		ClientIORate ClientIORate `json:"client_io_rate"`
	}

	Recovery map[string]interface{}

	RecoveryRate map[string]interface{}

	ClientIORate struct {
		ReadBytesSec  int64 `json:"read_bytes_sec"`
		WriteBytesSec int64 `json:"write_bytes_sec"`
		ReadOpPerSec  int64 `json:"read_op_per_sec"`
		WriteOpPerSec int64 `json:"write_op_per_sec"`
	}
)
