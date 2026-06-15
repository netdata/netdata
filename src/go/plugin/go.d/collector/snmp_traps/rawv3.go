// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"encoding/hex"
	"fmt"
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

// extractRawV3Context peeks at raw SNMPv3 data and returns the authoritative
// engine ID, username, and reportable flag without a full decode.
// Returns nil if the data is not a well-formed SNMPv3 message.
func extractRawV3Context(data []byte) (*rawV3Context, error) {
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

	pos := valueStart
	tag, intStart, intEnd, next, err := readBERElement(data[:valueEnd], pos)
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

	pos = next
	tag, gdStart, gdEnd, next, err := readBERElement(data[:valueEnd], pos)
	if err != nil {
		return nil, err
	}
	if tag != tagSequence {
		return nil, fmt.Errorf("SNMPv3 header data is not a sequence")
	}

	gdPos := gdStart

	// msgID
	tag, msgIDStart, msgIDEnd, gdPos, err := readBERElement(data[:gdEnd], gdPos)
	if err != nil {
		return nil, err
	}
	if tag != tagInteger {
		return nil, fmt.Errorf("SNMPv3 msgID is not an integer")
	}
	msgID, _ := parseBERUint32(data[msgIDStart:msgIDEnd])

	// msgMaxSize
	_, _, _, gdPos, err = readBERElement(data[:gdEnd], gdPos)
	if err != nil {
		return nil, err
	}

	// msgFlags (octet string, single byte)
	flagsTag, flagsStart, flagsEnd, gdPos, err := readBERElement(data[:gdEnd], gdPos)
	if err != nil {
		return nil, err
	}
	if flagsTag != tagOctetStr {
		return nil, fmt.Errorf("SNMPv3 msgFlags is not an octet string")
	}
	var reportable bool
	if flagsEnd-flagsStart == 1 {
		reportable = (data[flagsStart] & 0x04) != 0
	}

	// securityModel
	secModelTag, _, _, _, err := readBERElement(data[:gdEnd], gdPos)
	if err != nil {
		return nil, err
	}
	if secModelTag != tagInteger {
		return nil, fmt.Errorf("SNMPv3 securityModel is not an integer")
	}

	pos = next

	// securityParameters (octet string containing USM sequence)
	secTag, secStart, secEnd, _, err := readBERElement(data[:valueEnd], pos)
	if err != nil {
		return nil, err
	}
	if secTag != tagOctetStr {
		return nil, fmt.Errorf("SNMPv3 security parameters are not an octet string")
	}

	secData := data[secStart:secEnd]
	usmTag, usmStart, usmEnd, _, err := readBERElement(secData, 0)
	if err != nil {
		return nil, err
	}
	if usmTag != tagSequence {
		return nil, fmt.Errorf("SNMPv3 USM parameters are not a sequence")
	}

	usmPos := usmStart

	// authoritative engine ID
	aeTag, aeStart, aeEnd, usmPos, err := readBERElement(secData[:usmEnd], usmPos)
	if err != nil {
		return nil, err
	}
	if aeTag != tagOctetStr {
		return nil, fmt.Errorf("SNMPv3 authoritative engine ID is not an octet string")
	}
	engineID := hex.EncodeToString(secData[aeStart:aeEnd])

	// authoritative engine boots
	_, _, _, usmPos, err = readBERElement(secData[:usmEnd], usmPos)
	if err != nil {
		return nil, err
	}
	// authoritative engine time
	_, _, _, usmPos, err = readBERElement(secData[:usmEnd], usmPos)
	if err != nil {
		return nil, err
	}

	// userName
	unTag, unStart, unEnd, _, err := readBERElement(secData[:usmEnd], usmPos)
	if err != nil {
		return nil, err
	}
	if unTag != tagOctetStr {
		return nil, fmt.Errorf("SNMPv3 userName is not an octet string")
	}
	username := string(secData[unStart:unEnd])

	// authentication parameters (skip)
	// privacy parameters (skip)

	return &rawV3Context{
		engineID:   engineID,
		username:   username,
		reportable: reportable,
		msgID:      msgID,
	}, nil
}

func parseBERUint32(data []byte) (uint32, bool) {
	if len(data) == 0 || len(data) > 5 {
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
