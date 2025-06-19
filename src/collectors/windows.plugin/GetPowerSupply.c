// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

#define _COMMON_PLUGIN_NAME "windows.plugin"
#define _COMMON_PLUGIN_MODULE_NAME "GetPowerSupply"
#include "../common-contexts/common-contexts.h"

#define INITGUID
#include <windows.h>

#include <initguid.h>
#include <devguid.h>  // for GUID_DEVCLASS_BATTERY
#include <setupapi.h> // for SetupDi*
#include <batclass.h> // for BATTERY_*

static struct power_supply *power_supply_root = NULL;

static void netdata_allocate_power_supply(char *path)
{
    char name[RRD_ID_LENGTH_MAX + 1];
    strncpyz(name, path, RRD_ID_LENGTH_MAX);
    netdata_fix_chart_name(name);

    power_supply_root = callocz(1, sizeof(struct power_supply));
    power_supply_root->name = strdupz(name);

    power_supply_root->capacity = callocz(1, sizeof(struct simple_property));
    power_supply_root->capacity->filename = power_supply_root->name;
}

static void netdata_update_power_supply_values(BATTERY_STATUS *bs, BATTERY_INFORMATION *bi)
{
    if (bs->Capacity != BATTERY_UNKNOWN_CAPACITY) {
        collected_number num = bs->Capacity;
        collected_number den = bi->FullChargedCapacity;
        num /= den;

        power_supply_root->capacity->value = (unsigned long long)(num * 100.0);
    }
}

int do_GetPowerSupply(int update_every, usec_t dt __maybe_unused)
{
    PSP_DEVICE_INTERFACE_DETAIL_DATA pdidd = NULL;
    HANDLE hBattery = NULL;

    HDEVINFO hdev = SetupDiGetClassDevs(&GUID_DEVCLASS_BATTERY, 0, 0, DIGCF_PRESENT | DIGCF_DEVICEINTERFACE);
    if (hdev == INVALID_HANDLE_VALUE)
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

    if (!power_supply_root)
        netdata_allocate_power_supply(pdidd->DevicePath);

    netdata_update_power_supply_values(&bs, &bi);

    rrdset_create_simple_prop(
        power_supply_root,
        power_supply_root->capacity,
        "Battery capacity",
        "capacity",
        1,
        "percentage",
        NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY,
        update_every);

endPowerSupply:
    if (hBattery)
        CloseHandle(hBattery);

    if (pdidd)
        LocalFree(pdidd);

    if (hdev)
        SetupDiDestroyDeviceInfoList(hdev);

    return 0;
}
