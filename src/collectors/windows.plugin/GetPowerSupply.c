// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define INITGUID
#include <windows.h>

#include <initguid.h>
#include <devguid.h>   // for GUID_DEVCLASS_BATTERY
#include <setupapi.h>  // for SetupDi*
#include <batclass.h>  // for BATTERY_*

int do_GetPowerSupply(int update_every, usec_t dt __maybe_unused)
{
    PSP_DEVICE_INTERFACE_DETAIL_DATA pdidd = NULL;
    HANDLE hBattery = NULL;
    HDEVINFO hdev = SetupDiGetClassDevs(&GUID_DEVCLASS_BATTERY, 0, 0, DIGCF_PRESENT | DIGCF_DEVICEINTERFACE);
    if (hdev != INVALID_HANDLE_VALUE)
        return 1;

    SP_DEVICE_INTERFACE_DATA did = {0};
    did.cbSize = sizeof(did);

    if (!SetupDiEnumDeviceInterfaces(hdev, 0, &GUID_DEVCLASS_BATTERY, 0, &did))
        goto endPowerSupply;

    DWORD cbRequired = 0;
    SetupDiGetDeviceInterfaceDetail(hdev, &did, 0, 0, &cbRequired, 0);
    if (GetLastError() != ERROR_INSUFFICIENT_BUFFER) {
        goto endPowerSupply;
    }

    pdidd = (PSP_DEVICE_INTERFACE_DETAIL_DATA)LocalAlloc(LPTR, cbRequired);
    if (!pdidd)
        goto endPowerSupply;

    pdidd->cbSize = sizeof(*pdidd);
    if (!SetupDiGetDeviceInterfaceDetail(hdev, &did, pdidd, cbRequired, &cbRequired, 0)) {
        goto endPowerSupply;
    }

    hBattery = CreateFile(
        pdidd->DevicePath,
        GENERIC_READ | GENERIC_WRITE,
        FILE_SHARE_READ | FILE_SHARE_WRITE,
        NULL,
        OPEN_EXISTING,
        FILE_ATTRIBUTE_NORMAL,
        NULL);
    if (hBattery == INVALID_HANDLE_VALUE)
        goto endPowerSupply;

    BATTERY_QUERY_INFORMATION bqi = {0};

    DWORD dwWait = 0;
    DWORD dwOut;

    if (!DeviceIoControl(
            hBattery,
            IOCTL_BATTERY_QUERY_TAG,
            &dwWait,
            sizeof(dwWait),
            &bqi.BatteryTag,
            sizeof(bqi.BatteryTag),
            &dwOut,
            NULL) &&
        bqi.BatteryTag)
        goto endPowerSupply;

    BATTERY_INFORMATION bi = {0};
    bqi.InformationLevel = BatteryInformation;

    if (!DeviceIoControl(hBattery, IOCTL_BATTERY_QUERY_INFORMATION, &bqi, sizeof(bqi), &bi, sizeof(bi), &dwOut, NULL))
        goto endPowerSupply;

    BATTERY_WAIT_STATUS bws = {0};
    bws.BatteryTag = bqi.BatteryTag;

    BATTERY_STATUS bs;
    if (!DeviceIoControl(hBattery, IOCTL_BATTERY_QUERY_STATUS, &bws, sizeof(bws), &bs, sizeof(bs), &dwOut, NULL))
        goto endPowerSupply;

endPowerSupply:
    if (hBattery)
        CloseHandle(hBattery);

    if (pdidd)
        LocalFree(pdidd);

    if (hdev)
        SetupDiDestroyDeviceInfoList(hdev);

    return 0;
}
