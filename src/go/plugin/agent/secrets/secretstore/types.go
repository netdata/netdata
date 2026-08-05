// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

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

type ResolveRequest struct {
	StoreKey  string    `json:"store_key" yaml:"store_key"`
	StoreKind StoreKind `json:"store_kind" yaml:"store_kind"`
	StoreName string    `json:"store_name" yaml:"store_name"`
	Operand   string    `json:"operand" yaml:"operand"`
	Original  string    `json:"original" yaml:"original"`
}

var (
	ErrStoreNotFound = errStore("store is not configured")
)

type errStore string

func (e errStore) Error() string { return string(e) }

type JobRef struct {
	ID      string `json:"id" yaml:"id"`
	Display string `json:"display" yaml:"display"`
}
