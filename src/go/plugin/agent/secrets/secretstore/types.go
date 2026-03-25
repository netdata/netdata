// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import "time"

type StoreKind string

const (
	KindVault   StoreKind = "vault"
	KindAWSSM   StoreKind = "aws-sm"
	KindAzureKV StoreKind = "azure-kv"
	KindGCPSM   StoreKind = "gcp-sm"
)

func (k StoreKind) IsValid() bool {
	switch k {
	case KindVault, KindAWSSM, KindAzureKV, KindGCPSM:
		return true
	default:
		return false
	}
}

type ValidationStatus struct {
	CheckedAt time.Time `json:"checked_at" yaml:"checked_at"`
	OK        bool      `json:"ok" yaml:"ok"`
}

type StoreStatus struct {
	Name             string            `json:"name" yaml:"name"`
	Kind             StoreKind         `json:"kind" yaml:"kind"`
	LastValidation   *ValidationStatus `json:"last_validation,omitempty" yaml:"last_validation,omitempty"`
	LastErrorSummary string            `json:"last_error_summary,omitempty" yaml:"last_error_summary,omitempty"`
}

type ResolveRequest struct {
	StoreKey  string    `json:"store_key" yaml:"store_key"`
	StoreKind StoreKind `json:"store_kind" yaml:"store_kind"`
	StoreName string    `json:"store_name" yaml:"store_name"`
	Operand   string    `json:"operand" yaml:"operand"`
	Original  string    `json:"original" yaml:"original"`
}

var (
	ErrStoreExists   = errStore("store already exists")
	ErrStoreNotFound = errStore("store is not configured")
)

type errStore string

func (e errStore) Error() string { return string(e) }

type JobRef struct {
	ID      string `json:"id" yaml:"id"`
	Display string `json:"display" yaml:"display"`
}
