// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// ParsePCFResponse parses a PCF response message
func (c *Client) ParsePCFResponse(response []byte, command string) (map[C.MQLONG]interface{}, error) {
	attrs := make(map[C.MQLONG]interface{})

	if len(response) < int(C.sizeof_MQCFH) {
		return nil, fmt.Errorf("response too short for PCF header: got %d bytes, need at least %d", 
			len(response), C.sizeof_MQCFH)
	}

	cfh := (*C.MQCFH)(unsafe.Pointer(&response[0]))
	
	// Validate PCF header
	if cfh.Type != C.MQCFT_RESPONSE && cfh.Type != C.MQCFT_COMMAND && cfh.Type != C.MQCFT_STATISTICS {
		return nil, fmt.Errorf("unexpected PCF message type: %d", cfh.Type)
	}
	
	// Store the completion code and reason in the attributes for the caller to check
	attrs[C.MQIACF_COMP_CODE] = int32(cfh.CompCode)
	attrs[C.MQIACF_REASON_CODE] = int32(cfh.Reason)
	
	paramCount := int(cfh.ParameterCount)
	if paramCount < 0 || paramCount > 999 { // Sanity check
		c.protocol.Debugf("suspicious parameter count %d in PCF response", paramCount)
		return attrs, nil // Return what we have (comp/reason codes)
	}

	offset := int(C.sizeof_MQCFH)

	for i := 0; i < paramCount && offset < len(response); i++ {
		if offset+8 > len(response) { // Need at least 8 bytes for type + length
			c.protocol.Debugf("response truncated at parameter %d/%d", i+1, paramCount)
			break
		}

		paramType := *(*C.MQLONG)(unsafe.Pointer(&response[offset]))
		paramLength := *(*C.MQLONG)(unsafe.Pointer(&response[offset+4]))
		
		// Validate parameter length
		if paramLength < 8 || paramLength > C.MQLONG(len(response)-offset) {
			c.protocol.Debugf("invalid parameter length %d at offset %d", paramLength, offset)
			break
		}

		switch paramType {
		case C.MQCFT_INTEGER:
			if offset+int(C.sizeof_MQCFIN) > len(response) {
				c.protocol.Debugf("truncated MQCFIN at offset %d", offset)
				break
			}
			cfin := (*C.MQCFIN)(unsafe.Pointer(&response[offset]))
			if cfin.StrucLength != C.MQCFIN_STRUC_LENGTH {
				c.protocol.Debugf("unexpected MQCFIN length %d", cfin.StrucLength)
				offset += int(paramLength)
				continue
			}
			attrs[cfin.Parameter] = int32(cfin.Value)
			offset += int(cfin.StrucLength)

		case C.MQCFT_STRING:
			if offset+int(C.sizeof_MQCFST) > len(response) {
				c.protocol.Debugf("truncated MQCFST at offset %d", offset)
				break
			}
			cfst := (*C.MQCFST)(unsafe.Pointer(&response[offset]))
			if int(cfst.StrucLength) != int(paramLength) {
				c.protocol.Debugf("MQCFST length mismatch: header=%d, param=%d", cfst.StrucLength, paramLength)
				offset += int(paramLength)
				continue
			}

			// MQCFST structure is 20 bytes + string data
			stringDataStart := offset + 20
			stringDataEnd := stringDataStart + int(cfst.StringLength)
			if stringDataEnd > len(response) {
				c.protocol.Debugf("string data extends beyond response buffer")
				offset += int(paramLength)
				continue
			}
			
			// Ensure we don't read beyond the structure length
			if stringDataEnd > offset+int(cfst.StrucLength) {
				stringDataEnd = offset + int(cfst.StrucLength)
			}
			
			value := string(response[stringDataStart:stringDataEnd])
			trimmedValue := strings.TrimSpace(value)
			attrs[cfst.Parameter] = trimmedValue
			offset += int(cfst.StrucLength)

		case C.MQCFT_INTEGER64:
			if offset+int(C.sizeof_MQCFIN64) > len(response) {
				c.protocol.Debugf("truncated MQCFIN64 at offset %d", offset)
				break
			}
			cfin64 := (*C.MQCFIN64)(unsafe.Pointer(&response[offset]))
			if cfin64.StrucLength != C.MQCFIN64_STRUC_LENGTH {
				c.protocol.Debugf("unexpected MQCFIN64 length %d", cfin64.StrucLength)
				offset += int(paramLength)
				continue
			}
			attrs[cfin64.Parameter] = int64(cfin64.Value)
			offset += int(cfin64.StrucLength)

		case C.MQCFT_INTEGER_LIST:
			if offset+int(C.sizeof_MQCFIL) > len(response) {
				c.protocol.Debugf("truncated MQCFIL at offset %d", offset)
				break
			}
			cfil := (*C.MQCFIL)(unsafe.Pointer(&response[offset]))
			if int(cfil.StrucLength) != int(paramLength) {
				c.protocol.Debugf("MQCFIL length mismatch: header=%d, param=%d", cfil.StrucLength, paramLength)
				offset += int(paramLength)
				continue
			}
			
			// For now, we just skip integer lists as the original did
			// TODO: Parse integer list values if needed
			offset += int(cfil.StrucLength)

		case C.MQCFT_BYTE_STRING:
			// PCF parameter type 9 - MQCFT_BYTE_STRING
			if offset+int(C.sizeof_MQCFBS) > len(response) {
				c.protocol.Debugf("truncated MQCFBS at offset %d", offset)
				break
			}
			cfbs := (*C.MQCFBS)(unsafe.Pointer(&response[offset]))
			if int(cfbs.StrucLength) != int(paramLength) {
				c.protocol.Debugf("MQCFBS length mismatch: header=%d, param=%d", cfbs.StrucLength, paramLength)
				offset += int(paramLength)
				continue
			}

			c.protocol.Debugf("found MQCFBS %s (%d) with %d bytes of data at offset %d", 
				mqParameterToString(cfbs.Parameter), cfbs.Parameter, cfbs.StringLength, offset)
			
			// For now, skip byte string data as it's typically binary data not needed for monitoring
			// Could extract if needed: byteData := response[headerEnd:headerEnd+StringLength]
			offset += int(cfbs.StrucLength)

		case C.MQCFT_GROUP:
			// PCF parameter type 20 - MQCFT_GROUP
			// This is a group parameter that contains a nested structure of parameters
			if offset+int(C.sizeof_MQCFGR) > len(response) {
				c.protocol.Debugf("truncated MQCFGR at offset %d", offset)
				break
			}
			cfgr := (*C.MQCFGR)(unsafe.Pointer(&response[offset]))
			if int(cfgr.StrucLength) != int(paramLength) {
				c.protocol.Debugf("MQCFGR length mismatch: header=%d, param=%d", cfgr.StrucLength, paramLength)
				offset += int(paramLength)
				continue
			}

			c.protocol.Debugf("found MQCFGR group parameter %d with %d nested parameters at offset %d", 
				cfgr.Parameter, cfgr.ParameterCount, offset)
			
			// For now, skip the group structure as parsing nested parameters would be complex
			// Most monitoring metrics are not typically in groups
			offset += int(cfgr.StrucLength)

		case C.MQCFT_INTEGER64_LIST:
			// PCF parameter type 25 - MQCFT_INTEGER64_LIST  
			// This contains an array of 64-bit integer values
			if offset+int(C.sizeof_MQCFIL64) > len(response) {
				c.protocol.Debugf("truncated MQCFIL64 at offset %d", offset)
				break
			}
			cfil64 := (*C.MQCFIL64)(unsafe.Pointer(&response[offset]))
			if int(cfil64.StrucLength) != int(paramLength) {
				c.protocol.Debugf("MQCFIL64 length mismatch: header=%d, param=%d", cfil64.StrucLength, paramLength)
				offset += int(paramLength)
				continue
			}

			c.protocol.Debugf("found MQCFIL64 parameter %d with %d int64 values at offset %d", 
				cfil64.Parameter, cfil64.Count, offset)
			
			// For now, skip int64 list parsing as it's typically not needed for basic monitoring
			// Could extract values if needed for specific metrics
			offset += int(cfil64.StrucLength)

		default:
			// Unknown parameter type - skip it
			c.protocol.Debugf("unknown PCF parameter type %d at offset %d", paramType, offset)
			offset += int(paramLength)
		}
	}

	return attrs, nil
}