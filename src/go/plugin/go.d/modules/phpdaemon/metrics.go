// SPDX-License-Identifier: GPL-3.0-or-later

package phpdaemon

// https://github.com/kakserpom/phpdaemon/blob/master/PHPDaemon/Core/Daemon.php
// see getStateOfWorkers()

// WorkerState represents phpdaemon worker state.
type WorkerState struct {
	// Alive is sum of Idle, Busy and Reloading
	Alive    int64 `stm:"alive"`
	Shutdown int64 `stm:"shutdown"`

	// Idle that the worker is not in the middle of execution valuable callback (e.g. request) at this moment of time.
	// It does not mean that worker not have any pending operations.
	// Idle is sum of Preinit, Init and Initialized.
	Idle int64 `stm:"idle"`
	// Busy means that the worker is in the middle of execution valuable callback.
	Busy      int64 `stm:"busy"`
	Reloading int64 `stm:"reloading"`

	Preinit int64 `stm:"preinit"`
	// Init means that worker is starting right now.
	Init int64 `stm:"init"`
	// Initialized means that the worker is in Idle state.
	Initialized int64 `stm:"initialized"`
}

// FullStatus FullStatus.
type FullStatus struct {
	WorkerState `stm:""`
	Uptime      *int64 `stm:"uptime"`
}
