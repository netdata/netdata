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

// PCF parameter interface
type pcfParameter interface {
	size() C.size_t
	marshal(buffer unsafe.Pointer)
	logString() string
}

// String parameter
type stringParameter struct {
	parameter C.MQLONG
	value     string
}

func newStringParameter(param C.MQLONG, value string) pcfParameter {
	return &stringParameter{
		parameter: param,
		value:     value,
	}
}

func (p *stringParameter) size() C.size_t {
	// MQCFST structure size calculation matches C: sizeof(MQCFST) - sizeof(MQCHAR) + string length
	// The MQCFST struct already includes space for one MQCHAR, so we subtract it
	strLen := len(p.value)

	// Check if this is an MQ object name parameter - use correct sizes
	if objInfo := getObjectNameInfo(p.parameter); objInfo != nil {
		strLen = objInfo.padLength
	}

	paddedLen := (strLen + 3) & ^3 // Round up to multiple of 4
	return C.sizeof_MQCFST - C.sizeof_MQCHAR + C.size_t(paddedLen)
}

func (p *stringParameter) marshal(buffer unsafe.Pointer) {
	cfst := (*C.MQCFST)(buffer)
	cfst.Type = C.MQCFT_STRING
	cfst.StrucLength = C.MQLONG(p.size())
	cfst.Parameter = p.parameter
	cfst.CodedCharSetId = C.MQCCSI_DEFAULT

	// Check if this is an MQ object name parameter
	objInfo := getObjectNameInfo(p.parameter)
	if objInfo != nil {
		// StringLength is always the actual string length, not the padded length
		cfst.StringLength = C.MQLONG(len(p.value))

		// The String field is part of MQCFST structure, positioned after the fixed fields
		// Calculate offset: Type(4) + StrucLength(4) + Parameter(4) + CodedCharSetId(4) + StringLength(4) = 20 bytes
		stringData := uintptr(buffer) + 20
		C.memset(unsafe.Pointer(stringData), C.int(objInfo.fillChar), C.size_t(objInfo.padLength))

		// Copy the actual string
		actualLen := len(p.value)
		if actualLen > objInfo.padLength {
			actualLen = objInfo.padLength
		}
		if actualLen > 0 {
			cValue := C.CString(p.value)
			defer C.free(unsafe.Pointer(cValue))
			C.memcpy(unsafe.Pointer(stringData), unsafe.Pointer(cValue), C.size_t(actualLen))
		}
	} else {
		// For non-object strings, use the actual length
		cfst.StringLength = C.MQLONG(len(p.value))

		// Calculate padded length for non-object strings
		paddedLen := (len(p.value) + 3) & ^3

		// Zero-fill the entire padded area first (matching old behavior)
		// The String field is positioned after the fixed fields (20 bytes)
		stringData := uintptr(buffer) + 20
		C.memset(unsafe.Pointer(stringData), 0, C.size_t(paddedLen))

		// Copy string data
		if len(p.value) > 0 {
			cValue := C.CString(p.value)
			defer C.free(unsafe.Pointer(cValue))
			C.memcpy(unsafe.Pointer(stringData), unsafe.Pointer(cValue), C.size_t(len(p.value)))
		}
	}
}

func (p *stringParameter) logString() string {
	paramName := mqParameterToString(p.parameter)
	value := p.value
	// Truncate long values for readability
	if len(value) > 50 {
		value = value[:47] + "..."
	}
	return fmt.Sprintf("%s=%s", paramName, value)
}

// Integer parameter
type intParameter struct {
	parameter C.MQLONG
	value     int32
}

func newIntParameter(param C.MQLONG, value int32) pcfParameter {
	return &intParameter{
		parameter: param,
		value:     value,
	}
}

func (p *intParameter) size() C.size_t {
	return C.sizeof_MQCFIN
}

func (p *intParameter) marshal(buffer unsafe.Pointer) {
	cfin := (*C.MQCFIN)(buffer)
	cfin.Type = C.MQCFT_INTEGER
	cfin.StrucLength = C.MQCFIN_STRUC_LENGTH
	cfin.Parameter = p.parameter
	cfin.Value = C.MQLONG(p.value)
}

func (p *intParameter) logString() string {
	paramName := mqParameterToString(p.parameter)
	return fmt.Sprintf("%s=%d", paramName, p.value)
}

// String filter parameter
type stringFilterParameter struct {
	parameter C.MQLONG
	value     string
	operator  C.MQLONG
}

func newStringFilterParameter(param C.MQLONG, value string, operator C.MQLONG) pcfParameter {
	return &stringFilterParameter{
		parameter: param,
		value:     value,
		operator:  operator,
	}
}

func (p *stringFilterParameter) size() C.size_t {
	// MQCFSF structure size calculation matches C: sizeof(MQCFSF) - sizeof(MQCHAR) + string length
	// The MQCFSF struct already includes space for one MQCHAR, so we subtract it
	strLen := len(p.value)

	// Check if this is an MQ object name parameter - use correct sizes
	if objInfo := getObjectNameInfo(p.parameter); objInfo != nil {
		strLen = objInfo.padLength
	}

	paddedLen := (strLen + 3) & ^3 // Round up to multiple of 4
	return C.sizeof_MQCFSF - C.sizeof_MQCHAR + C.size_t(paddedLen)
}

func (p *stringFilterParameter) marshal(buffer unsafe.Pointer) {
	cfsf := (*C.MQCFSF)(buffer)
	cfsf.Type = C.MQCFT_STRING_FILTER
	cfsf.StrucLength = C.MQLONG(p.size())
	cfsf.Parameter = p.parameter
	cfsf.CodedCharSetId = C.MQCCSI_DEFAULT
	cfsf.Operator = p.operator

	// Determine string length and fill character
	strLen := len(p.value)
	fillChar := byte(0) // Default: zero-fill
	
	if objInfo := getObjectNameInfo(p.parameter); objInfo != nil {
		// This is an MQ object name parameter - use correct padding
		strLen = objInfo.padLength
		fillChar = objInfo.fillChar
	}

	// Calculate the actual buffer size for the string data (must match size())
	paddedLen := (strLen + 3) & ^3

	// The String field is part of MQCFSF structure, positioned after the fixed fields
	// MQCFSF: Type(4) + StrucLength(4) + Parameter(4) + CodedCharSetId(4) + Operator(4) = 20 bytes
	stringDataPtr := unsafe.Pointer(uintptr(buffer) + 20)

	// Fill the entire padded area with the appropriate character
	C.memset(stringDataPtr, C.int(fillChar), C.size_t(paddedLen))

	// Convert Go string to byte slice
	goBytes := []byte(p.value)

	// Copy the actual string value bytes (up to the maximum length)
	if len(goBytes) > 0 {
		copyLen := len(goBytes)
		if copyLen > strLen {
			copyLen = strLen
		}
		C.memcpy(stringDataPtr, unsafe.Pointer(&goBytes[0]), C.size_t(copyLen))
	}
	
	// Note: The old code did not explicitly set FilterValueLength
	// Keeping exact compatibility by not setting it
}

func (p *stringFilterParameter) logString() string {
	paramName := mqParameterToString(p.parameter)
	operatorStr := mqOperatorToString(p.operator)
	value := p.value
	// Truncate long values for readability
	if len(value) > 50 {
		value = value[:47] + "..."
	}
	return fmt.Sprintf("%s=%s(%s)", paramName, value, operatorStr)
}

// Integer list parameter
type intListParameter struct {
	parameter C.MQLONG
	values    []int32
}

func newIntListParameter(param C.MQLONG, values []int32) pcfParameter {
	return &intListParameter{
		parameter: param,
		values:    values,
	}
}

func (p *intListParameter) size() C.size_t {
	// MQCFIL: Type(4) + StrucLength(4) + Parameter(4) + Count(4) + Values(4*Count)
	return 16 + C.size_t(len(p.values)*4)
}

func (p *intListParameter) marshal(buffer unsafe.Pointer) {
	cfil := (*C.MQCFIL)(buffer)
	cfil.Type = C.MQCFT_INTEGER_LIST
	cfil.StrucLength = C.MQLONG(p.size())
	cfil.Parameter = p.parameter
	cfil.Count = C.MQLONG(len(p.values))

	// Copy integer values
	if len(p.values) > 0 {
		valuesPtr := unsafe.Pointer(uintptr(buffer) + 16)
		for i, val := range p.values {
			*(*C.MQLONG)(unsafe.Pointer(uintptr(valuesPtr) + uintptr(i*4))) = C.MQLONG(val)
		}
	}
}

func (p *intListParameter) logString() string {
	paramName := mqParameterToString(p.parameter)
	// Create string representation of values
	valueStrs := make([]string, len(p.values))
	for i, val := range p.values {
		valueStrs[i] = fmt.Sprintf("%d", val)
	}
	// Truncate long lists for readability
	valueStr := strings.Join(valueStrs, ",")
	if len(p.values) > 10 {
		valueStr = strings.Join(valueStrs[:10], ",") + ",..."
	}
	return fmt.Sprintf("%s=[%s]", paramName, valueStr)
}