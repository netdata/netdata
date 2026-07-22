// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

// FramePublicationPort writes Function-registration protocol frames. The
// Publication state machine owns the set of currently published Functions.
type FramePublicationPort struct {
	frames *lifecycle.FrameOwner // the one serialized stdout frame writer
}

func NewFramePublicationPort(frames *lifecycle.FrameOwner) (*FramePublicationPort, error) {
	if frames == nil {
		return nil, errors.New("jobmgr Function protocol: invalid publication port")
	}
	return &FramePublicationPort{frames: frames}, nil
}

func (fpp *FramePublicationPort) Publish(record PublicationRecord) error {
	if fpp == nil {
		return errors.New("jobmgr Function protocol: nil publication port")
	}
	payload, err := encodeFunctionRegistration(record)
	if err != nil {
		return err
	}
	prepared, err := lifecycle.PrepareProtocolFrame(payload)
	if err != nil {
		return err
	}
	return fpp.frames.CommitPreparedProtocolFrame(prepared)
}

func (fpp *FramePublicationPort) Withdraw(name string) error {
	if fpp == nil {
		return errors.New("jobmgr Function protocol: invalid withdrawal")
	}
	payload, err := encodeFunctionWithdrawal(name)
	if err != nil {
		return err
	}
	prepared, err := lifecycle.PrepareProtocolFrame(payload)
	if err != nil {
		return err
	}
	return fpp.frames.CommitPreparedProtocolFrame(prepared)
}

func encodeFunctionRegistration(record PublicationRecord) ([]byte, error) {
	if !record.validCore() || !validFunctionName(record.Name) ||
		!validQuotedProtocolField(record.Help) ||
		!validQuotedProtocolField(record.Tags) ||
		!validFunctionAccess(record.Access) {
		return nil, errors.New("jobmgr Function protocol: invalid registration")
	}
	payload := make([]byte, 0, len(record.Name)+len(record.Help)+len(record.Tags)+80)
	payload = append(payload, "FUNCTION GLOBAL "...)
	payload = appendQuotedFunctionName(payload, record.Name)
	payload = append(payload, ' ')
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
	payload := make([]byte, 0, len(name)+24)
	payload = append(payload, "FUNCTION_DEL GLOBAL "...)
	payload = appendQuotedFunctionName(payload, name)
	payload = append(payload, '\n', '\n')
	return payload, nil
}

func appendQuotedFunctionName(payload []byte, name string) []byte {
	payload = append(payload, '"')
	payload = append(payload, name...)
	return append(payload, '"')
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
