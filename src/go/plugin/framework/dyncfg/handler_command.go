// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/framework/functions"
)

// Prepare performs rejection-capable work against an immutable entry snapshot.
// Apply must run on the component's serialized state owner.
func (h *Handler[C]) Prepare(fn Function) (PreparedCommand, error) {
	if fn.Context() != nil && fn.Context().Err() != nil {
		return nil, context.Cause(fn.Context())
	}
	command := fn.Command()
	key, name, ok := h.cb.ExtractKey(fn)
	var expected *Entry[C]
	if ok {
		expected, _ = h.exposed.LookupByKey(key)
	}
	reject := func(code int, message string) (PreparedCommand, error) {
		return &preparedHandlerCommand{
			apply: func() (AppliedCommand, error) {
				result := handlerCommandResult(fn, code, message)
				if expected != nil {
					current, _ := h.exposed.LookupByKey(key)
					if current == expected {
						result.Notifications = []Notification{h.statusNotification(expected.Cfg, expected.Status)}
					}
				}
				return result, nil
			},
		}, nil
	}
	if command == CommandAdd {
		var code int
		var msg string
		key, name, code, msg = h.addRejection(fn)
		if code != 0 {
			return reject(code, msg)
		}
		expected, _ = h.exposed.LookupByKey(key)
	} else {
		if !ok {
			return reject(400, "invalid config ID format.")
		}
		if expected == nil {
			return reject(404, "config not found.")
		}
	}
	var next C
	var activation PreparedActivation
	switch command {
	case CommandAdd, CommandUpdate:
		if err := fn.ValidateHasPayload(); err != nil {
			return reject(400, err.Error())
		}
		var err error
		next, err = h.cb.ParseAndValidate(fn, name)
		if err != nil {
			return reject(callbackErrorCode(err, 400), err.Error())
		}
		if next.ExposedKey() != key {
			return reject(400, "configuration identity differs from the requested name.")
		}
		if command == CommandAdd && h.cb.ConfigType(next) != ConfigTypeJob {
			return reject(
				405,
				fmt.Sprintf(
					"adding configurations of type '%s' is not supported, only 'job' configurations can be added.",
					h.cb.ConfigType(next),
				),
			)
		}
		if err := h.ValidateConfigCreate(next, StatusAccepted); err != nil {
			return reject(
				400,
				fmt.Sprintf("configuration cannot be represented in the plugins.d CONFIG protocol: %v", err),
			)
		}
		if command == CommandUpdate {
			if expected.Status == StatusAccepted && !expected.Enabled {
				return reject(403, "updating is not allowed in 'accepted' state.")
			}
			identical := expected.Status == StatusRunning && expected.Cfg.SourceType() == "dyncfg" &&
				expected.Cfg.Hash() == next.Hash()
			if !identical && expected.Status != StatusDisabled {
				activation, err = h.cb.PrepareUpdate(fn, expected.Cfg, next)
				if err != nil {
					return reject(callbackErrorCode(err, 422), err.Error())
				}
				if activation == nil {
					return nil, errors.New("dyncfg: update preparation returned no activation")
				}
			}
		}
	case CommandEnable:
		switch expected.Status {
		case StatusAccepted, StatusDisabled, StatusFailed, StatusRunning:
		default:
			return reject(405, fmt.Sprintf("enabling is not allowed in '%s' state.", expected.Status))
		}
	case CommandDisable:
	case CommandRemove:
		if expected.Cfg.SourceType() != "dyncfg" {
			return reject(
				405,
				fmt.Sprintf(
					"removing configurations of source type '%s' is not supported, only 'dyncfg' configurations can be removed.",
					expected.Cfg.SourceType(),
				),
			)
		}
		if h.cb.ConfigType(expected.Cfg) != ConfigTypeJob {
			return reject(
				405,
				fmt.Sprintf(
					"removing configurations of type '%s' is not supported, only 'job' configurations can be removed.",
					h.cb.ConfigType(expected.Cfg),
				),
			)
		}
	default:
		return reject(501, fmt.Sprintf("Command '%s' is not implemented.", command))
	}
	dispose := func() {
		if activation != nil {
			activation.Dispose()
			activation = nil
		}
	}
	return &preparedHandlerCommand{dispose: dispose, apply: func() (AppliedCommand, error) {
		current, _ := h.exposed.LookupByKey(key)
		if current != expected {
			return handlerCommandResult(fn, 409, "configuration changed during preparation; retry the command."), nil
		}
		if command == CommandEnable || command == CommandDisable {
			h.SyncDecision(fn)
		}
		switch command {
		case CommandAdd:
			if expected != nil {
				h.cb.Stop(expected.Cfg)
				// Replay may replace a waiting file config with a different origin key.
				h.waitGate.replace(expected.Cfg, next)
				if expected.Cfg.SourceType() == "dyncfg" {
					h.seen.Remove(expected.Cfg)
				}
			}
			h.seen.Add(next)
			h.exposed.Add(&Entry[C]{
				Cfg:    next,
				Status: StatusAccepted,
			})
			result := handlerCommandResult(fn, 202, "")
			result.Notifications = []Notification{
				{Kind: NotificationCreate, Config: h.configCreateOpts(next, StatusAccepted)},
			}
			return result, nil
		case CommandEnable:
			if expected.Status == StatusRunning {
				return h.statusResult(fn, expected.Cfg, StatusRunning, 200), nil
			}
			if expected.Status == StatusAccepted && expected.Enabled {
				return h.statusResult(fn, expected.Cfg, StatusAccepted, 202), nil
			}
			published, err := h.cb.Enable(expected.Cfg)
			if err != nil {
				return AppliedCommand{}, err
			}
			entry := &Entry[C]{
				Cfg:     expected.Cfg,
				Status:  StatusAccepted,
				Enabled: true,
			}
			h.exposed.Add(entry)
			h.cb.OnStatusChange(entry, expected.Status, fn)
			result := h.statusResult(fn, entry.Cfg, entry.Status, 202)
			result.Published = published
			return result, nil
		case CommandDisable:
			if expected.Status != StatusDisabled {
				h.cb.Stop(expected.Cfg)
			}
			entry := &Entry[C]{
				Cfg:    expected.Cfg,
				Status: StatusDisabled,
			}
			h.exposed.Add(entry)
			if expected.Status != StatusDisabled {
				h.cb.OnStatusChange(entry, expected.Status, fn)
			}
			return h.statusResult(fn, entry.Cfg, entry.Status, 200), nil
		case CommandRemove:
			h.cb.Stop(expected.Cfg)
			h.waitGate.clearIfMatch(h.waitGate.keyFor(expected.Cfg))
			h.seen.Remove(expected.Cfg)
			h.exposed.Remove(expected.Cfg)
			result := handlerCommandResult(fn, 200, "")
			result.Notifications = []Notification{{Kind: NotificationDelete, ID: h.cb.ConfigID(expected.Cfg)}}
			return result, nil
		case CommandUpdate:
			if expected.Status == StatusRunning && expected.Cfg.SourceType() == "dyncfg" &&
				expected.Cfg.Hash() == next.Hash() {
				return h.statusResult(fn, expected.Cfg, StatusRunning, 200), nil
			}
			var published func()
			status, code := StatusDisabled, 200
			if expected.Status != StatusDisabled {
				var err error
				published, err = activation.Accept()
				if err != nil {
					return AppliedCommand{}, err
				}
				activation = nil // Successful acceptance transfers ownership to the component.
				status, code = StatusAccepted, 202
			}
			if expected.Cfg.SourceType() == "dyncfg" {
				h.seen.Remove(expected.Cfg)
			}
			h.seen.Add(next)
			entry := &Entry[C]{
				Cfg:     next,
				Status:  status,
				Enabled: status != StatusDisabled,
			}
			h.exposed.Add(entry)
			h.cb.OnStatusChange(entry, expected.Status, fn)
			result := h.statusResult(fn, next, status, code)
			result.Published = published
			if expected.Cfg.SourceType() != "dyncfg" {
				result.Notifications = append(
					[]Notification{{Kind: NotificationCreate, Config: h.configCreateOpts(next, status)}},
					result.Notifications...)
			}
			return result, nil
		}
		return AppliedCommand{}, errors.New("dyncfg: invalid prepared command")
	}}, nil
}

func handlerCommandResult(fn Function, code int, message string) AppliedCommand {
	return AppliedCommand{
		Result: Result{
			UID:         fn.UID(),
			Code:        code,
			ContentType: "application/json",
			Payload:     string(functions.BuildJSONPayload(code, message)),
		},
	}
}
func (h *Handler[C]) statusNotification(cfg C, status Status) Notification {
	return Notification{
		Kind:   NotificationStatus,
		ID:     h.cb.ConfigID(cfg),
		Status: status,
	}
}
func (h *Handler[C]) statusResult(fn Function, cfg C, status Status, code int) AppliedCommand {
	result := handlerCommandResult(fn, code, "")
	result.Notifications = []Notification{h.statusNotification(cfg, status)}
	return result
}

type preparedHandlerCommand struct {
	mu       sync.Mutex
	consumed bool
	apply    func() (AppliedCommand, error)
	dispose  func()
}

func (p *preparedHandlerCommand) take() (func() (AppliedCommand, error), func(), error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.consumed {
		return nil, nil, errors.New("dyncfg: prepared command consumed")
	}
	p.consumed = true
	apply, dispose := p.apply, p.dispose
	p.apply = nil
	p.dispose = nil
	return apply, dispose, nil
}
func (p *preparedHandlerCommand) Apply(ctx context.Context) (AppliedCommand, error) {
	apply, dispose, err := p.take()
	if err != nil {
		return AppliedCommand{}, err
	}
	if dispose != nil {
		defer dispose()
	}
	if ctx == nil {
		return AppliedCommand{}, errors.New("dyncfg: nil apply context")
	}
	if ctx.Err() != nil {
		return AppliedCommand{}, context.Cause(ctx)
	}
	return apply()
}
func (p *preparedHandlerCommand) Dispose(context.Context) error {
	_, dispose, err := p.take()
	if err != nil {
		return err
	}
	if dispose != nil {
		dispose()
	}
	return nil
}
