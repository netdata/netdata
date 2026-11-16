// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// CommandPreparer performs reject-capable work without adopting configuration.
// The caller owns the returned command and must apply or dispose it exactly once.
type CommandPreparer func(Function) (PreparedCommand, error)

// PreparedRegistry separates mutation preparation from Function invocation.
// Read-only commands continue to use the ordinary prefix handler.
type PreparedRegistry interface {
	functions.Registry
	RegisterCommandPreparer(name, prefix string, prepare CommandPreparer)
}

// PreparedCommand holds an unaccepted command. Apply performs only serialized
// state adoption and logical retirement; it must not wait for physical work.
type PreparedCommand interface {
	Apply(context.Context) (AppliedCommand, error)
	Dispose(context.Context) error
}

// AppliedCommand owns its reply and notifications. Published releases the exact
// accepted continuation only after both have been written successfully. The caller
// invokes it at most once; the component releases activation on its serialized
// state owner. Failed publication must fail that owner closed without activation.
type AppliedCommand struct {
	Result        Result
	Notifications []Notification
	Published     func()
}

type NotificationKind uint8

const (
	NotificationCreate NotificationKind = iota + 1
	NotificationStatus
	NotificationDelete
)

// Notification is owned command output, independent of an active invocation.
type Notification struct {
	Kind   NotificationKind
	Config netdataapi.ConfigOpts
	ID     string
	Status Status
}

func (n Notification) Validate() error {
	switch n.Kind {
	case NotificationCreate:
		return n.Config.Validate()
	case NotificationStatus, NotificationDelete:
		if !netdataapi.ValidBareProtocolField(n.ID) {
			return errors.New("invalid configuration notification ID")
		}
		if n.Kind == NotificationStatus {
			switch n.Status {
			case StatusAccepted, StatusRunning, StatusFailed, StatusIncomplete, StatusDisabled:
			default:
				return errors.New("invalid configuration notification status")
			}
		}
		return nil
	default:
		return errors.New("invalid configuration notification kind")
	}
}

func (n Notification) Emit(output Output) {
	switch n.Kind {
	case NotificationCreate:
		output.ConfigCreate(n.Config)
	case NotificationStatus:
		output.ConfigStatus(n.ID, n.Status)
	case NotificationDelete:
		output.ConfigDelete(n.ID)
	}
}

// PreparedActivation is a component-owned UPDATE preflight result. Accept
// transfers it to the desired-state owner and returns its publication release.
// Accept must be bounded and non-blocking on physical work. On error it must
// preserve the incumbent and leave the result owned by the caller for Dispose.
// Dispose drops an unaccepted result without disturbing the incumbent.
type PreparedActivation interface {
	Accept() (published func(), err error)
	Dispose()
}
