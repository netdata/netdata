//go:build unix

package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/netipc/protocol"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/netipc/service/cgroups"
)

const defaultSharedSnapshotServiceName = "ebpf-cgroups-snapshot"

type SharedSnapshotItem struct {
	Hash    uint32
	Options uint32
	Enabled uint32
	Name    string
	Path    string
}

type SharedSnapshotState struct {
	SystemdEnabled uint32
	Generation     uint64
	Items          []SharedSnapshotItem
}

type SharedSnapshotStore struct {
	mu    sync.RWMutex
	state SharedSnapshotState
}

func NewSharedSnapshotStore() *SharedSnapshotStore {
	return &SharedSnapshotStore{}
}

func (s *SharedSnapshotStore) Replace(state SharedSnapshotState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	copied := make([]SharedSnapshotItem, len(state.Items))
	copy(copied, state.Items)
	state.Items = copied
	s.state = state
}

func (s *SharedSnapshotStore) Snapshot() SharedSnapshotState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copied := make([]SharedSnapshotItem, len(s.state.Items))
	copy(copied, s.state.Items)

	return SharedSnapshotState{
		SystemdEnabled: s.state.SystemdEnabled,
		Generation:     s.state.Generation,
		Items:          copied,
	}
}

type SharedSnapshotService struct {
	store  *SharedSnapshotStore
	server *cgroups.Server
}

func defaultSharedSnapshotServerConfig() cgroups.ServerConfig {
	return cgroups.ServerConfig{
		SupportedProfiles:       protocol.ProfileBaseline | protocol.ProfileSHMHybrid | protocol.ProfileSHMFutex,
		PreferredProfiles:       protocol.ProfileSHMFutex,
		MaxRequestBatchItems:    1,
		MaxResponsePayloadBytes: 65536,
		AuthToken:               0,
	}
}

func NewSharedSnapshotService(runDir, serviceName string, config cgroups.ServerConfig, store *SharedSnapshotStore) *SharedSnapshotService {
	if store == nil {
		store = NewSharedSnapshotStore()
	}

	handler := cgroups.Handler{
		Handle: func(req *protocol.CgroupsRequest, builder *protocol.CgroupsBuilder) bool {
			_ = req
			return writeSharedSnapshot(builder, store.Snapshot())
		},
	}

	return &SharedSnapshotService{
		store: store,
		server: cgroups.NewServerWithWorkers(
			runDir,
			serviceName,
			config,
			handler,
			2,
		),
	}
}

func writeSharedSnapshot(builder *protocol.CgroupsBuilder, snapshot SharedSnapshotState) bool {
	builder.SetHeader(snapshot.SystemdEnabled, snapshot.Generation)

	for _, item := range snapshot.Items {
		if err := builder.Add(item.Hash, item.Options, item.Enabled, []byte(item.Name), []byte(item.Path)); err != nil {
			return false
		}
	}

	return true
}

func (s *SharedSnapshotService) Run() error {
	return s.server.Run()
}

func (s *SharedSnapshotService) Stop() {
	s.server.Stop()
}

func runSharedSnapshotService() {
	service := NewSharedSnapshotService(
		"/var/run/netdata",
		defaultSharedSnapshotServiceName,
		defaultSharedSnapshotServerConfig(),
		nil,
	)

	if service == nil {
		return
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = service.Run()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)

	service.Stop()
	<-done
}
