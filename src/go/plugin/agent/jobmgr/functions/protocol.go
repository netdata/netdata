// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

// FramePublicationPort is the sole Function-registration protocol writer.
// Handles become live only after the complete frame has been committed.
type FramePublicationPort struct {
	mu sync.Mutex

	epoch  uint64
	nextID uint64
	frames *lifecycle.FrameOwner
	active map[uint64]PublicationHandle
}

func NewFramePublicationPort(
	epoch uint64,
	frames *lifecycle.FrameOwner,
) (*FramePublicationPort, error) {
	if epoch == 0 || frames == nil {
		return nil, errors.New("jobmgr Function protocol: invalid publication port")
	}
	return &FramePublicationPort{
		epoch: epoch, frames: frames, active: make(map[uint64]PublicationHandle),
	}, nil
}

func (fpp *FramePublicationPort) Publish(
	record PublicationRecord,
) (PublicationHandle, error) {
	if fpp == nil {
		return PublicationHandle{}, errors.New("jobmgr Function protocol: nil publication port")
	}
	payload, err := encodeFunctionRegistration(record)
	if err != nil {
		return PublicationHandle{}, err
	}
	prepared, err := lifecycle.PrepareProtocolFrame(payload)
	if err != nil {
		return PublicationHandle{}, err
	}
	fpp.mu.Lock()
	defer fpp.mu.Unlock()
	nextID := fpp.nextID + 1
	if nextID == 0 {
		return PublicationHandle{}, errors.New("jobmgr Function protocol: handle identity wrapped")
	}
	if err := fpp.frames.CommitPreparedProtocolFrame(prepared); err != nil {
		return PublicationHandle{}, err
	}
	fpp.nextID = nextID
	handle := PublicationHandle{
		ID: nextID, Epoch: fpp.epoch,
		Generation: record.Generation, Name: record.Name,
	}
	fpp.active[handle.ID] = handle
	return handle, nil
}

func (fpp *FramePublicationPort) Withdraw(handle PublicationHandle) error {
	if fpp == nil || handle.ID == 0 || handle.Epoch != fpp.epoch ||
		handle.Generation == 0 || handle.Name == "" {
		return errors.New("jobmgr Function protocol: invalid withdrawal")
	}
	payload, err := encodeFunctionWithdrawal(handle.Name)
	if err != nil {
		return err
	}
	prepared, err := lifecycle.PrepareProtocolFrame(payload)
	if err != nil {
		return err
	}
	fpp.mu.Lock()
	defer fpp.mu.Unlock()
	current, exists := fpp.active[handle.ID]
	if !exists || current != handle {
		return errors.New("jobmgr Function protocol: stale or cross-generation withdrawal")
	}
	if err := fpp.frames.CommitPreparedProtocolFrame(prepared); err != nil {
		return err
	}
	delete(fpp.active, handle.ID)
	return nil
}

func encodeFunctionRegistration(record PublicationRecord) ([]byte, error) {
	if record.Generation == 0 || record.Timeout < 0 || record.Priority < 0 ||
		record.Version < 0 || !validFunctionName(record.Name) ||
		!validQuotedProtocolField(record.Help) ||
		!validQuotedProtocolField(record.Tags) ||
		!validFunctionAccess(record.Access) {
		return nil, errors.New("jobmgr Function protocol: invalid registration")
	}
	payload := make([]byte, 0, len(record.Name)+len(record.Help)+len(record.Tags)+80)
	payload = append(payload, `FUNCTION GLOBAL "`...)
	payload = append(payload, record.Name...)
	payload = append(payload, `" `...)
	payload = strconv.AppendInt(payload, int64(record.Timeout), 10)
	payload = append(payload, ` "`...)
	payload = append(payload, record.Help...)
	payload = append(payload, `" "`...)
	payload = append(payload, record.Tags...)
	payload = append(payload, `" `...)
	payload = append(payload, record.Access...)
	payload = append(payload, ' ')
	payload = strconv.AppendInt(payload, int64(record.Priority), 10)
	payload = append(payload, ' ')
	payload = strconv.AppendInt(payload, int64(record.Version), 10)
	payload = append(payload, '\n', '\n')
	return payload, nil
}

func encodeFunctionWithdrawal(name string) ([]byte, error) {
	if !validFunctionName(name) {
		return nil, errors.New("jobmgr Function protocol: invalid withdrawal name")
	}
	return fmt.Appendf(nil, "FUNCTION_DEL GLOBAL %q\n\n", name), nil
}

func validFunctionName(value string) bool {
	if value == "" || strings.TrimSpace(value) != value {
		return false
	}
	for _, char := range value {
		if char <= ' ' || char == '"' || char == '\\' || char == 0x7f {
			return false
		}
	}
	return utf8.ValidString(value)
}

func validQuotedProtocolField(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, char := range value {
		if char == '"' || char == '\\' || char < ' ' || char == 0x7f {
			return false
		}
	}
	return true
}

func validFunctionAccess(value string) bool {
	if len(value) != 6 || value[0] != '0' || value[1] != 'x' {
		return false
	}
	for _, char := range value[2:] {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
