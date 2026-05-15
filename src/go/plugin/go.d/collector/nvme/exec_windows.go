// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package nvme

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unsafe"

	"github.com/netdata/netdata/go/plugins/logger"

	"golang.org/x/sys/windows"
)

const (
	maxWindowsPhysicalDrives = 256

	ioctlStorageQueryProperty = 0x002d1400

	storageDeviceProperty                 = 0
	storageDeviceProtocolSpecificProperty = 50
	propertyStandardQuery                 = 0

	busTypeNvme       = 0x11
	protocolTypeNvme  = 3
	nvmeDataLogPage   = 2
	nvmeHealthLogPage = 0x02
	nvmeHealthLogSize = 512

	storagePropertyQueryHeaderSize    = 8
	storagePropertyQuerySize          = 12
	storageProtocolSpecificDataSize   = 40
	storageProtocolDataDescriptorSize = 48
)

type windowsNvmeCliExec struct {
	*logger.Logger

	native   nvmeCli
	fallback nvmeCli
}

func (n *windowsNvmeCliExec) list() (*nvmeDeviceList, error) {
	devList, err := n.native.list()
	if err == nil && len(devList.Devices) > 0 {
		return devList, nil
	}

	if n.fallback != nil {
		if err != nil {
			n.Debugf("native Windows NVMe discovery failed, trying nvme CLI fallback: %v", err)
		}
		return n.fallback.list()
	}

	return devList, err
}

func (n *windowsNvmeCliExec) smartLog(devicePath string) (*nvmeDeviceSmartLog, error) {
	if isWindowsPhysicalDrivePath(devicePath) {
		return n.native.smartLog(devicePath)
	}

	if n.fallback != nil {
		return n.fallback.smartLog(devicePath)
	}

	return nil, fmt.Errorf("unsupported Windows NVMe device path %q", devicePath)
}

type windowsNvmeExec struct {
	*logger.Logger
}

type windowsNvmeDevice struct {
	path   string
	model  string
	serial string
}

type storageDeviceDescriptor struct {
	version               uint32
	size                  uint32
	deviceType            byte
	deviceTypeModifier    byte
	removableMedia        byte
	commandQueueing       byte
	vendorIDOffset        uint32
	productIDOffset       uint32
	productRevisionOffset uint32
	serialNumberOffset    uint32
	busType               uint32
	rawPropertiesLength   uint32
}

func newWindowsNvmeExec(log *logger.Logger) *windowsNvmeExec {
	return &windowsNvmeExec{Logger: log}
}

func (e *windowsNvmeExec) list() (*nvmeDeviceList, error) {
	devices, err := e.listDevices()
	if err != nil {
		return nil, err
	}

	var list nvmeDeviceList
	for _, dev := range devices {
		list.Devices = append(list.Devices, struct {
			DevicePath   string `json:"DevicePath"`
			Firmware     string `json:"Firmware"`
			ModelNumber  string `json:"ModelNumber"`
			SerialNumber string `json:"SerialNumber"`
		}{
			DevicePath:   dev.path,
			ModelNumber:  dev.model,
			SerialNumber: dev.serial,
		})
	}

	return &list, nil
}

func (e *windowsNvmeExec) smartLog(devicePath string) (*nvmeDeviceSmartLog, error) {
	handle, err := openWindowsPhysicalDrive(devicePath)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(handle)

	logPage, err := queryWindowsNvmeHealthLog(handle)
	if err != nil {
		return nil, err
	}

	return windowsNvmeHealthLogToSmartLog(logPage)
}

func (e *windowsNvmeExec) listDevices() ([]windowsNvmeDevice, error) {
	var devices []windowsNvmeDevice
	var lastErr error

	for i := 0; i < maxWindowsPhysicalDrives; i++ {
		path := fmt.Sprintf(`\\.\PhysicalDrive%d`, i)

		handle, err := openWindowsPhysicalDrive(path)
		if err != nil {
			if shouldIgnorePhysicalDriveOpenError(err) {
				continue
			}
			lastErr = err
			continue
		}

		desc, raw, err := queryWindowsStorageDeviceDescriptor(handle)
		_ = windows.CloseHandle(handle)
		if err != nil {
			lastErr = err
			continue
		}
		if desc.busType != busTypeNvme {
			continue
		}

		devices = append(devices, windowsNvmeDevice{
			path:   path,
			model:  stringFromDescriptorOffset(raw, desc.productIDOffset),
			serial: stringFromDescriptorOffset(raw, desc.serialNumberOffset),
		})
	}

	if len(devices) == 0 && lastErr != nil {
		return nil, lastErr
	}

	return devices, nil
}

func openWindowsPhysicalDrive(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}

	var lastErr error
	for _, access := range []uint32{0, windows.GENERIC_READ, windows.GENERIC_READ | windows.GENERIC_WRITE} {
		handle, err := windows.CreateFile(
			name,
			access,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil,
			windows.OPEN_EXISTING,
			windows.FILE_ATTRIBUTE_NORMAL,
			0,
		)
		if err == nil {
			return handle, nil
		}
		lastErr = err
	}

	return windows.InvalidHandle, lastErr
}

func shouldIgnorePhysicalDriveOpenError(err error) bool {
	return err == windows.ERROR_FILE_NOT_FOUND ||
		err == windows.ERROR_PATH_NOT_FOUND ||
		err == windows.ERROR_INVALID_NAME ||
		err == windows.ERROR_INVALID_PARAMETER
}

func queryWindowsStorageDeviceDescriptor(handle windows.Handle) (storageDeviceDescriptor, []byte, error) {
	query := newWindowsStoragePropertyQuery(storageDeviceProperty)

	out := make([]byte, 4096)
	var returned uint32
	if err := windows.DeviceIoControl(
		handle,
		ioctlStorageQueryProperty,
		&query[0],
		uint32(len(query)),
		&out[0],
		uint32(len(out)),
		&returned,
		nil,
	); err != nil {
		return storageDeviceDescriptor{}, nil, err
	}

	if returned < uint32(unsafe.Sizeof(storageDeviceDescriptor{})) {
		return storageDeviceDescriptor{}, nil, fmt.Errorf("short storage device descriptor: %d bytes", returned)
	}

	out = out[:returned]
	desc := storageDeviceDescriptor{
		version:               binary.LittleEndian.Uint32(out[0:4]),
		size:                  binary.LittleEndian.Uint32(out[4:8]),
		deviceType:            out[8],
		deviceTypeModifier:    out[9],
		removableMedia:        out[10],
		commandQueueing:       out[11],
		vendorIDOffset:        binary.LittleEndian.Uint32(out[12:16]),
		productIDOffset:       binary.LittleEndian.Uint32(out[16:20]),
		productRevisionOffset: binary.LittleEndian.Uint32(out[20:24]),
		serialNumberOffset:    binary.LittleEndian.Uint32(out[24:28]),
		busType:               binary.LittleEndian.Uint32(out[28:32]),
		rawPropertiesLength:   binary.LittleEndian.Uint32(out[32:36]),
	}

	return desc, out, nil
}

func queryWindowsNvmeHealthLog(handle windows.Handle) ([]byte, error) {
	buf := make([]byte, storagePropertyQueryHeaderSize+storageProtocolSpecificDataSize+nvmeHealthLogSize)

	binary.LittleEndian.PutUint32(buf[0:4], storageDeviceProtocolSpecificProperty)
	binary.LittleEndian.PutUint32(buf[4:8], propertyStandardQuery)

	protocolData := buf[storagePropertyQueryHeaderSize:]
	binary.LittleEndian.PutUint32(protocolData[0:4], protocolTypeNvme)
	binary.LittleEndian.PutUint32(protocolData[4:8], nvmeDataLogPage)
	binary.LittleEndian.PutUint32(protocolData[8:12], nvmeHealthLogPage)
	binary.LittleEndian.PutUint32(protocolData[16:20], storageProtocolSpecificDataSize)
	binary.LittleEndian.PutUint32(protocolData[20:24], nvmeHealthLogSize)

	var returned uint32
	if err := windows.DeviceIoControl(
		handle,
		ioctlStorageQueryProperty,
		&buf[0],
		uint32(len(buf)),
		&buf[0],
		uint32(len(buf)),
		&returned,
		nil,
	); err != nil {
		return nil, err
	}

	if returned < storageProtocolDataDescriptorSize {
		return nil, fmt.Errorf("short NVMe protocol descriptor: %d bytes", returned)
	}

	outProtocolData := buf[storagePropertyQueryHeaderSize:]
	dataOffset := binary.LittleEndian.Uint32(outProtocolData[16:20])
	dataLength := binary.LittleEndian.Uint32(outProtocolData[20:24])
	if dataOffset < storageProtocolSpecificDataSize || dataLength < nvmeHealthLogSize {
		return nil, fmt.Errorf("invalid NVMe health log descriptor: offset=%d length=%d", dataOffset, dataLength)
	}

	start := storagePropertyQueryHeaderSize + int(dataOffset)
	end := start + nvmeHealthLogSize
	if end > len(buf) || uint32(end) > returned {
		return nil, fmt.Errorf("short NVMe health log: offset=%d returned=%d", start, returned)
	}

	return buf[start:end], nil
}

func newWindowsStoragePropertyQuery(propertyID uint32) []byte {
	// STORAGE_PROPERTY_QUERY has an 8-byte fixed header plus a 1-byte
	// AdditionalParameters field padded by the ABI; Windows rejects shorter
	// descriptor queries with ERROR_BAD_LENGTH on some storage stacks.
	query := make([]byte, storagePropertyQuerySize)
	binary.LittleEndian.PutUint32(query[0:4], propertyID)
	binary.LittleEndian.PutUint32(query[4:8], propertyStandardQuery)
	return query
}

func windowsNvmeHealthLogToSmartLog(logPage []byte) (*nvmeDeviceSmartLog, error) {
	if len(logPage) < nvmeHealthLogSize {
		return nil, fmt.Errorf("short NVMe health log: %d bytes", len(logPage))
	}

	return &nvmeDeviceSmartLog{
		CriticalWarningValue: nvmeNumber(fmt.Sprintf("%d", logPage[0])),
		Temperature:          nvmeNumber(fmt.Sprintf("%d", binary.LittleEndian.Uint16(logPage[1:3]))),
		AvailSpare:           nvmeNumber(fmt.Sprintf("%d", logPage[3])),
		SpareThresh:          nvmeNumber(fmt.Sprintf("%d", logPage[4])),
		PercentUsed:          nvmeNumber(fmt.Sprintf("%d", logPage[5])),
		DataUnitsRead:        nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64Max(logPage[32:48], math.MaxInt64/(1000*512)))),
		DataUnitsWritten:     nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64Max(logPage[48:64], math.MaxInt64/(1000*512)))),
		HostReadCommands:     nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64(logPage[64:80]))),
		HostWriteCommands:    nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64(logPage[80:96]))),
		ControllerBusyTime:   nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64Max(logPage[96:112], math.MaxInt64/60))),
		PowerCycles:          nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64(logPage[112:128]))),
		PowerOnHours:         nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64Max(logPage[128:144], math.MaxInt64/3600))),
		UnsafeShutdowns:      nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64(logPage[144:160]))),
		MediaErrors:          nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64(logPage[160:176]))),
		NumErrLogEntries:     nvmeNumber(fmt.Sprintf("%d", littleEndianUint128ToInt64(logPage[176:192]))),
		WarningTempTime:      nvmeNumber(fmt.Sprintf("%d", binary.LittleEndian.Uint32(logPage[192:196]))),
		CriticalCompTime:     nvmeNumber(fmt.Sprintf("%d", binary.LittleEndian.Uint32(logPage[196:200]))),
		ThmTemp1TransCount:   nvmeNumber(fmt.Sprintf("%d", binary.LittleEndian.Uint32(logPage[216:220]))),
		ThmTemp2TransCount:   nvmeNumber(fmt.Sprintf("%d", binary.LittleEndian.Uint32(logPage[220:224]))),
		ThmTemp1TotalTime:    nvmeNumber(fmt.Sprintf("%d", binary.LittleEndian.Uint32(logPage[224:228]))),
		ThmTemp2TotalTime:    nvmeNumber(fmt.Sprintf("%d", binary.LittleEndian.Uint32(logPage[228:232]))),
	}, nil
}

func littleEndianUint128ToInt64(b []byte) int64 {
	return littleEndianUint128ToInt64Max(b, math.MaxInt64)
}

func littleEndianUint128ToInt64Max(b []byte, maxValue int64) int64 {
	if len(b) < 16 {
		return 0
	}

	for _, v := range b[8:16] {
		if v != 0 {
			return maxValue
		}
	}

	v := binary.LittleEndian.Uint64(b[:8])
	if v > uint64(maxValue) {
		return maxValue
	}

	return int64(v)
}

func stringFromDescriptorOffset(buf []byte, offset uint32) string {
	if offset == 0 || int(offset) >= len(buf) {
		return ""
	}

	s := string(buf[offset:])
	if idx := strings.IndexByte(s, 0); idx >= 0 {
		s = s[:idx]
	}

	return strings.TrimSpace(s)
}
