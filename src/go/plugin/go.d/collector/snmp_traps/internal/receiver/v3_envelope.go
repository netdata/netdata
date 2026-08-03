// SPDX-License-Identifier: GPL-3.0-or-later

package receiver

import (
	"encoding/hex"
	"fmt"
)

const (
	maxSNMPv3Integer = uint32(1<<31 - 1)
	minV3MsgMaxSize  = uint32(484)
	usmSecurityModel = uint32(3)
)

// rawV3Context holds fields extracted from raw SNMPv3 data before full
// decode: the authoritative engine ID (hex), the USM security name
// (username), and whether the message is reportable.
type rawV3Context struct {
	engineID   string
	username   string
	reportable bool
	msgID      uint32
}

type rawV3Envelope struct {
	headerData         []byte
	securityParameters []byte
}

// extractRawV3Context peeks at raw SNMPv3 data and returns the authoritative
// engine ID, username, and reportable flag without a full decode.
// Returns nil if the data is not a well-formed SNMPv3 message.
func extractRawV3Context(data []byte) (*rawV3Context, error) {
	envelope, err := parseRawV3Envelope(data)
	if err != nil || envelope == nil {
		return nil, err
	}
	msgID, reportable, err := parseRawV3Header(envelope.headerData)
	if err != nil {
		return nil, err
	}
	engineID, username, err := parseRawUSMContext(envelope.securityParameters, true)
	if err != nil {
		return nil, err
	}

	return &rawV3Context{
		engineID:   engineID,
		username:   username,
		reportable: reportable,
		msgID:      msgID,
	}, nil
}

func parseRawV3Envelope(data []byte) (*rawV3Envelope, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("too short for SNMPv3")
	}

	tag, valueStart, valueEnd, _, err := readBERElement(data, 0)
	if err != nil {
		return nil, err
	}
	if tag != tagSequence {
		return nil, nil
	}

	tag, intStart, intEnd, next, err := readBERElement(data[:valueEnd], valueStart)
	if err != nil {
		return nil, err
	}
	if tag != tagInteger {
		return nil, nil
	}
	version, ok := parseBERVersion(data[intStart:intEnd])
	if !ok || version != 3 {
		return nil, nil
	}

	tag, headerStart, headerEnd, next, err := readBERElement(data[:valueEnd], next)
	if err != nil {
		return nil, err
	}
	if tag != tagSequence {
		return nil, fmt.Errorf("SNMPv3 header data is not a sequence")
	}

	tag, securityStart, securityEnd, _, err := readBERElement(data[:valueEnd], next)
	if err != nil {
		return nil, err
	}
	if tag != tagOctetStr {
		return nil, fmt.Errorf("SNMPv3 security parameters are not an octet string")
	}

	return &rawV3Envelope{
		headerData:         data[headerStart:headerEnd],
		securityParameters: data[securityStart:securityEnd],
	}, nil
}

func parseRawV3Header(data []byte) (msgID uint32, reportable bool, err error) {
	tag, valueStart, valueEnd, next, err := readBERElement(data, 0)
	if err != nil {
		return 0, false, err
	}
	if tag != tagInteger {
		return 0, false, fmt.Errorf("SNMPv3 msgID is not an integer")
	}
	msgID, ok := parseBERUint32(data[valueStart:valueEnd])
	if !ok || msgID > maxSNMPv3Integer {
		return 0, false, fmt.Errorf("SNMPv3 msgID is invalid")
	}

	tag, valueStart, valueEnd, next, err = readBERElement(data, next)
	if err != nil {
		return 0, false, err
	}
	if tag != tagInteger {
		return 0, false, fmt.Errorf("SNMPv3 msgMaxSize is not an integer")
	}
	msgMaxSize, ok := parseBERUint32(data[valueStart:valueEnd])
	if !ok || msgMaxSize < minV3MsgMaxSize || msgMaxSize > maxSNMPv3Integer {
		return 0, false, fmt.Errorf("SNMPv3 msgMaxSize is invalid")
	}

	tag, valueStart, valueEnd, next, err = readBERElement(data, next)
	if err != nil {
		return 0, false, err
	}
	if tag != tagOctetStr {
		return 0, false, fmt.Errorf("SNMPv3 msgFlags is not an octet string")
	}
	if valueEnd-valueStart != 1 {
		return 0, false, fmt.Errorf("SNMPv3 msgFlags must contain exactly one byte")
	}
	flags := data[valueStart]
	if flags&^byte(0x07) != 0 || flags&0x03 == 0x02 {
		return 0, false, fmt.Errorf("SNMPv3 msgFlags is invalid")
	}
	reportable = flags&0x04 != 0

	tag, valueStart, valueEnd, next, err = readBERElement(data, next)
	if err != nil {
		return 0, false, err
	}
	if tag != tagInteger {
		return 0, false, fmt.Errorf("SNMPv3 securityModel is not an integer")
	}
	securityModel, ok := parseBERUint32(data[valueStart:valueEnd])
	if !ok || securityModel != usmSecurityModel {
		return 0, false, fmt.Errorf("SNMPv3 securityModel is not USM")
	}
	if next != len(data) {
		return 0, false, fmt.Errorf("SNMPv3 header data contains trailing fields")
	}
	return msgID, reportable, nil
}

func parseRawUSMContext(data []byte, includeUsername bool) (engineID, username string, err error) {
	tag, valueStart, valueEnd, outerNext, err := readBERElement(data, 0)
	if err != nil {
		return "", "", err
	}
	if tag != tagSequence {
		return "", "", fmt.Errorf("SNMPv3 USM parameters are not a sequence")
	}
	sequenceEnd := valueEnd

	tag, engineStart, engineEnd, next, err := readBERElement(data[:sequenceEnd], valueStart)
	if err != nil {
		return "", "", err
	}
	if tag != tagOctetStr {
		return "", "", fmt.Errorf("SNMPv3 authoritative engine ID is not an octet string")
	}
	engineID = hex.EncodeToString(data[engineStart:engineEnd])
	if !includeUsername {
		return engineID, "", nil
	}

	tag, valueStart, valueEnd, next, err = readBERElement(data[:sequenceEnd], next)
	if err != nil {
		return "", "", err
	}
	if tag != tagInteger {
		return "", "", fmt.Errorf("SNMPv3 authoritative engine boots is not an integer")
	}
	if value, ok := parseBERUint32(data[valueStart:valueEnd]); !ok || value > maxSNMPv3Integer {
		return "", "", fmt.Errorf("SNMPv3 authoritative engine boots is invalid")
	}

	tag, valueStart, valueEnd, next, err = readBERElement(data[:sequenceEnd], next)
	if err != nil {
		return "", "", err
	}
	if tag != tagInteger {
		return "", "", fmt.Errorf("SNMPv3 authoritative engine time is not an integer")
	}
	if value, ok := parseBERUint32(data[valueStart:valueEnd]); !ok || value > maxSNMPv3Integer {
		return "", "", fmt.Errorf("SNMPv3 authoritative engine time is invalid")
	}

	tag, valueStart, valueEnd, next, err = readBERElement(data[:sequenceEnd], next)
	if err != nil {
		return "", "", err
	}
	if tag != tagOctetStr {
		return "", "", fmt.Errorf("SNMPv3 userName is not an octet string")
	}
	username = string(data[valueStart:valueEnd])

	tag, _, _, next, err = readBERElement(data[:sequenceEnd], next)
	if err != nil {
		return "", "", err
	}
	if tag != tagOctetStr {
		return "", "", fmt.Errorf("SNMPv3 authentication parameters are not an octet string")
	}

	tag, _, _, next, err = readBERElement(data[:sequenceEnd], next)
	if err != nil {
		return "", "", err
	}
	if tag != tagOctetStr {
		return "", "", fmt.Errorf("SNMPv3 privacy parameters are not an octet string")
	}
	if next != sequenceEnd || outerNext != len(data) {
		return "", "", fmt.Errorf("SNMPv3 USM parameters contain trailing fields")
	}
	return engineID, username, nil
}

func parseBERUint32(data []byte) (uint32, bool) {
	if len(data) == 0 || len(data) > 5 {
		return 0, false
	}
	if data[0]&0x80 != 0 || len(data) == 5 && data[0] != 0 {
		return 0, false
	}
	var v uint64
	for _, b := range data {
		v = (v << 8) | uint64(b)
	}
	if v > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(v), true
}

func (ctx *rawV3Context) discoveryProbe() bool {
	if ctx == nil || !ctx.reportable {
		return false
	}
	if ctx.engineID == "" {
		return true
	}
	_, err := parseEngineIDHex(ctx.engineID)
	return err != nil
}
