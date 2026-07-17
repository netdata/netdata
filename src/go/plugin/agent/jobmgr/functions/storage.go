// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"sync/atomic"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

const (
	MaximumCatalogStorageBytes = int64(
		lifecycle.OrdinaryBudgetBytes - lifecycle.MaximumFunctionFrameBytes,
	)
	// The node charge includes a conservative allowance for one individually
	// allocated object's allocator metadata and alignment.
	catalogNodeStorageBytes = int64(unsafe.Sizeof(catalogNode{})) + 16
	prefixNodeStorageBytes  = int64(unsafe.Sizeof(prefixNode{})) + 16
	pointerStorageBytes     = int64(unsafe.Sizeof((*catalogNode)(nil)))
)

type catalogStorage struct {
	published   atomic.Int64
	total       atomic.Int64
	preparation atomic.Bool
}

func (storage *catalogStorage) initialize(published int64) error {
	if storage == nil || published < 0 ||
		published > MaximumCatalogStorageBytes ||
		!storage.published.CompareAndSwap(0, published) ||
		!storage.total.CompareAndSwap(0, published) {
		return errors.New("jobmgr Function catalog: invalid initial path storage")
	}
	return nil
}

func (storage *catalogStorage) reservePreparation(bytes int64) error {
	if storage == nil || bytes <= 0 ||
		bytes > MaximumCatalogStorageBytes ||
		!storage.preparation.CompareAndSwap(false, true) {
		return errors.New(
			"jobmgr Function catalog: mutation path storage is unavailable",
		)
	}
	total := storage.total.Load()
	if total < 0 || bytes > MaximumCatalogStorageBytes-total {
		storage.preparation.Store(false)
		return errors.New(
			"jobmgr Function catalog: mutation path storage exceeds process bound",
		)
	}
	storage.total.Add(bytes)
	return nil
}

func (storage *catalogStorage) discardPreparation(bytes int64) error {
	if storage == nil || bytes <= 0 ||
		!storage.preparation.CompareAndSwap(true, false) {
		return errors.New(
			"jobmgr Function catalog: invalid mutation storage discard",
		)
	}
	if total := storage.total.Add(-bytes); total != storage.published.Load() {
		return errors.New(
			"jobmgr Function catalog: mutation storage accounting differs",
		)
	}
	return nil
}

func (storage *catalogStorage) publishPreparation(
	preparedBytes int64,
	publishedBytes int64,
) error {
	if storage == nil || preparedBytes <= 0 ||
		publishedBytes < 0 ||
		publishedBytes > MaximumCatalogStorageBytes ||
		!storage.preparation.CompareAndSwap(true, false) {
		return errors.New(
			"jobmgr Function catalog: invalid mutation storage publication",
		)
	}
	previous := storage.published.Swap(publishedBytes)
	if total := storage.total.Add(
		publishedBytes - previous - preparedBytes,
	); total != publishedBytes {
		return errors.New(
			"jobmgr Function catalog: published storage accounting differs",
		)
	}
	return nil
}

func (storage *catalogStorage) releasePublished() error {
	if storage == nil || storage.preparation.Load() {
		return errors.New(
			"jobmgr Function catalog: invalid published storage release",
		)
	}
	published := storage.published.Swap(0)
	if total := storage.total.Add(-published); total != 0 {
		return errors.New(
			"jobmgr Function catalog: final path storage accounting differs",
		)
	}
	return nil
}

func initialPathStorageBound(declarations []Declaration) (int64, error) {
	var total int64
	for _, declaration := range declarations {
		nameNodes := int64(len(declaration.PublicName))*8 + 1
		if err := addStorageProduct(
			&total,
			nameNodes,
			catalogNodeStorageBytes,
		); err != nil {
			return 0, err
		}
		if declaration.Prefix != "" {
			prefixNodes := int64(len(declaration.Prefix))*8 + 1
			if err := addStorageProduct(
				&total,
				prefixNodes*2,
				prefixNodeStorageBytes,
			); err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

func mutationPathStorageBound(changes []RouteChange) (int64, error) {
	var total int64
	for _, change := range changes {
		nameBits := int64(len(change.PublicName)) * 8
		if err := addMutationPathStorage(
			&total,
			nameBits,
			catalogNodeStorageBytes,
		); err != nil {
			return 0, err
		}
		if change.Prefix != "" {
			prefixBits := int64(len(change.Prefix)) * 8
			if err := addMutationPathStorage(
				&total,
				prefixBits,
				prefixNodeStorageBytes,
			); err != nil {
				return 0, err
			}
		}
	}
	return total, nil
}

func addMutationPathStorage(
	total *int64,
	bits int64,
	nodeBytes int64,
) error {
	nodes := bits + 1
	perNode := pointerStorageBytes*3 + nodeBytes*2
	if err := addStorageProduct(total, nodes, perNode); err != nil {
		return err
	}
	return addStorageProduct(total, bits, 1)
}

func addStorageProduct(total *int64, count int64, bytes int64) error {
	if total == nil || count < 0 || bytes < 0 ||
		(count != 0 && bytes > MaximumCatalogStorageBytes/count) ||
		*total > MaximumCatalogStorageBytes-count*bytes {
		return errors.New(
			"jobmgr Function catalog: path storage exceeds process bound",
		)
	}
	*total += count * bytes
	return nil
}

func catalogPathStorage(root *catalogNode) int64 {
	if root == nil {
		return 0
	}
	total := catalogNodeStorageBytes
	if root.routes.prefixes != nil {
		total += prefixPathStorage(root.routes.prefixes)
	}
	total += catalogPathStorage(root.child[0])
	total += catalogPathStorage(root.child[1])
	return total
}

func prefixPathStorage(root *prefixNode) int64 {
	if root == nil {
		return 0
	}
	return prefixNodeStorageBytes +
		prefixPathStorage(root.child[0]) +
		prefixPathStorage(root.child[1])
}
