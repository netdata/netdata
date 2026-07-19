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
	// Retired generations remain catalog-owned until cleanup acknowledgement.
	catalogGenerationRetentionBytes = int64(4 * 1024)
	// The node charge includes a conservative allowance for one individually
	// allocated object's allocator metadata and alignment.
	catalogNodeStorageBytes = int64(unsafe.Sizeof(catalogNode{})) + 16
	prefixNodeStorageBytes  = int64(unsafe.Sizeof(prefixNode{})) + 16
	pointerStorageBytes     = int64(unsafe.Sizeof((*catalogNode)(nil)))
)

type catalogStorage struct {
	published   atomic.Int64
	cleanup     atomic.Int64
	total       atomic.Int64
	preparation atomic.Bool
}

func (cs *catalogStorage) initialize(
	published int64,
	cleanup int64,
) error {
	if cs == nil || published < 0 || cleanup < 0 ||
		published > MaximumCatalogStorageBytes-cleanup ||
		!cs.published.CompareAndSwap(0, published) ||
		!cs.cleanup.CompareAndSwap(0, cleanup) ||
		!cs.total.CompareAndSwap(0, published+cleanup) {
		return errors.New("jobmgr Function catalog: invalid initial path storage")
	}
	return nil
}

func (cs *catalogStorage) reservePreparation(bytes int64) error {
	if cs == nil || bytes <= 0 ||
		bytes > MaximumCatalogStorageBytes ||
		!cs.preparation.CompareAndSwap(false, true) {
		return errors.New(
			"jobmgr Function catalog: mutation path storage is unavailable",
		)
	}
	total := cs.total.Load()
	if total < 0 || bytes > MaximumCatalogStorageBytes-total {
		cs.preparation.Store(false)
		return errors.New(
			"jobmgr Function catalog: mutation path storage exceeds process bound",
		)
	}
	cs.total.Add(bytes)
	return nil
}

func (cs *catalogStorage) discardPreparation(bytes int64) error {
	if cs == nil || bytes <= 0 ||
		!cs.preparation.Load() {
		return errors.New(
			"jobmgr Function catalog: invalid mutation storage discard",
		)
	}
	if !subtractStorage(&cs.total, bytes) ||
		!cs.preparation.CompareAndSwap(true, false) {
		return errors.New(
			"jobmgr Function catalog: mutation storage accounting differs",
		)
	}
	return nil
}

func (cs *catalogStorage) publishPreparation(
	preparedPathBytes int64,
	preparedCleanupBytes int64,
	publishedBytes int64,
) error {
	if cs == nil || preparedPathBytes <= 0 ||
		preparedCleanupBytes < 0 ||
		publishedBytes < 0 ||
		publishedBytes > MaximumCatalogStorageBytes-
			cs.cleanup.Load()-preparedCleanupBytes ||
		!cs.preparation.Load() {
		return errors.New(
			"jobmgr Function catalog: invalid mutation storage publication",
		)
	}
	previous := cs.published.Load()
	if !adjustStorage(
		&cs.total,
		publishedBytes-previous-preparedPathBytes,
	) {
		return errors.New(
			"jobmgr Function catalog: published storage accounting differs",
		)
	}
	cs.cleanup.Add(preparedCleanupBytes)
	cs.published.Store(publishedBytes)
	if !cs.preparation.CompareAndSwap(true, false) {
		return errors.New(
			"jobmgr Function catalog: mutation storage publication lost ownership",
		)
	}
	return nil
}

func (cs *catalogStorage) abortPreparation(
	preparedBytes int64,
	retainedCleanupBytes int64,
) error {
	if cs == nil || preparedBytes <= 0 ||
		retainedCleanupBytes < 0 ||
		retainedCleanupBytes > preparedBytes ||
		!cs.preparation.Load() {
		return errors.New(
			"jobmgr Function catalog: invalid mutation storage abort",
		)
	}
	if !subtractStorage(
		&cs.total,
		preparedBytes-retainedCleanupBytes,
	) {
		return errors.New(
			"jobmgr Function catalog: aborted storage accounting differs",
		)
	}
	cs.cleanup.Add(retainedCleanupBytes)
	if !cs.preparation.CompareAndSwap(true, false) {
		return errors.New(
			"jobmgr Function catalog: mutation storage abort lost ownership",
		)
	}
	return nil
}

func (cs *catalogStorage) releasePublished() error {
	if cs == nil || cs.preparation.Load() {
		return errors.New(
			"jobmgr Function catalog: invalid published storage release",
		)
	}
	published := cs.published.Swap(0)
	if !subtractStorage(&cs.total, published) {
		return errors.New(
			"jobmgr Function catalog: final path storage accounting differs",
		)
	}
	return nil
}

func (cs *catalogStorage) releasePublishedPaths(bytes int64) {
	if cs == nil || bytes <= 0 {
		return
	}
	cs.published.Add(-bytes)
	cs.total.Add(-bytes)
}

func (cs *catalogStorage) releaseCleanup(bytes int64) error {
	if cs == nil || bytes <= 0 {
		return errors.New(
			"jobmgr Function catalog: invalid cleanup storage release",
		)
	}
	if cleanup := cs.cleanup.Add(-bytes); cleanup < 0 {
		cs.cleanup.Add(bytes)
		return errors.New(
			"jobmgr Function catalog: cleanup storage underflow",
		)
	}
	if !subtractStorage(&cs.total, bytes) {
		cs.cleanup.Add(bytes)
		return errors.New(
			"jobmgr Function catalog: cleanup storage accounting differs",
		)
	}
	return nil
}

func adjustStorage(storage *atomic.Int64, delta int64) bool {
	if storage == nil {
		return false
	}
	if delta < 0 {
		return subtractStorage(storage, -delta)
	}
	if delta == 0 {
		return true
	}
	if total := storage.Add(delta); total > MaximumCatalogStorageBytes {
		storage.Add(-delta)
		return false
	}
	return true
}

func subtractStorage(storage *atomic.Int64, bytes int64) bool {
	if storage == nil || bytes < 0 {
		return false
	}
	if bytes == 0 {
		return true
	}
	for {
		current := storage.Load()
		if current < bytes {
			return false
		}
		if storage.CompareAndSwap(current, current-bytes) {
			return true
		}
	}
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
