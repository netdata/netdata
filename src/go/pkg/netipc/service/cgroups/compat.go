// Package cgroups preserves the historical import path for the
// cgroups-snapshot service.
//
// New code should prefer package cgroups_snapshot.
package cgroups

import snapshot "github.com/netdata/netdata/go/plugins/pkg/netipc/service/cgroups_snapshot"

type ClientState = snapshot.ClientState

const (
	StateDisconnected = snapshot.StateDisconnected
	StateConnecting   = snapshot.StateConnecting
	StateReady        = snapshot.StateReady
	StateNotFound     = snapshot.StateNotFound
	StateAuthFailed   = snapshot.StateAuthFailed
	StateIncompatible = snapshot.StateIncompatible
	StateBroken       = snapshot.StateBroken
)

type ClientStatus = snapshot.ClientStatus
type ClientConfig = snapshot.ClientConfig
type ServerConfig = snapshot.ServerConfig
type SnapshotHandler = snapshot.SnapshotHandler
type Handler = snapshot.Handler
type Client = snapshot.Client
type Server = snapshot.Server
type Cache = snapshot.Cache
type CacheItem = snapshot.CacheItem
type CacheStatus = snapshot.CacheStatus

var NewClient = snapshot.NewClient
var NewServer = snapshot.NewServer
var NewServerWithWorkers = snapshot.NewServerWithWorkers
var NewCache = snapshot.NewCache
