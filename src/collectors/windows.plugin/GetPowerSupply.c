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

static inline void netdata_allocate_power_supply(char *path)
{
    power_supply_root = callocz(1, sizeof(struct power_supply));
    power_supply_root->capacity = callocz(1, sizeof(struct simple_property));
}

static inline void netdata_update_power_supply_values(
    HANDLE hBattery,
    struct simple_property *voltage,
    BATTERY_INFORMATION *bi,
    BATTERY_QUERY_INFORMATION *bqi)
{
    BATTERY_WAIT_STATUS bws = {0};
    bws.BatteryTag = bqi->BatteryTag;
    DWORD dwOut;

    BATTERY_STATUS bs;
    if (!DeviceIoControl(hBattery, IOCTL_BATTERY_QUERY_STATUS, &bws, sizeof(bws), &bs, sizeof(bs), &dwOut, NULL))
        return;

    if (bs.Capacity != BATTERY_UNKNOWN_CAPACITY) {
        NETDATA_DOUBLE num = bs.Capacity;
        NETDATA_DOUBLE den = bi->FullChargedCapacity;
        num = (den) ? num/ den : 0;

        power_supply_root->capacity->value = (unsigned long long)(num * 100.0);
    }

    if (bs.Voltage != BATTERY_UNKNOWN_VOLTAGE) {
        voltage->value = bs.Voltage;
    }
}

static void netdata_power_supply_plot(struct simple_property *voltage, int update_every)
{
    rrdset_create_simple_prop(
        power_supply_root,
        power_supply_root->capacity,
        "Battery capacity",
        "capacity",
        1,
        "percentage",
        NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY,
        update_every);

    rrdset_create_simple_prop(
        power_supply_root,
        voltage,
        "Power supply voltage",
        "now",
        1000,
        "v",
        NETDATA_CHART_PRIO_POWER_SUPPLY_VOLTAGE,
        update_every);
}

int do_GetPowerSupply(int update_every, usec_t dt __maybe_unused)
{
    static struct simple_property voltage;

    HDEVINFO hdev = SetupDiGetClassDevs(&GUID_DEVCLASS_BATTERY, 0, 0, DIGCF_PRESENT | DIGCF_DEVICEINTERFACE);
    if (hdev == INVALID_HANDLE_VALUE)
        return 1;

    SP_DEVICE_INTERFACE_DATA did = {0};
    did.cbSize = sizeof(did);

    for (LONG i = 0; i < 32 && SetupDiEnumDeviceInterfaces(hdev, 0, &GUID_DEVCLASS_BATTERY, i, &did); i++) {
        DWORD cbRequired = 0;
        PSP_DEVICE_INTERFACE_DETAIL_DATA pdidd = NULL;
        HANDLE hBattery = NULL;
        SetupDiGetDeviceInterfaceDetail(hdev, &did, 0, 0, &cbRequired, 0);
        if (GetLastError() != ERROR_INSUFFICIENT_BUFFER)
            goto endPowerSupply;

        pdidd = (PSP_DEVICE_INTERFACE_DETAIL_DATA)LocalAlloc(LPTR, cbRequired);
        if (!pdidd)
            goto endPowerSupply;

        pdidd->cbSize = sizeof(*pdidd);
        if (!SetupDiGetDeviceInterfaceDetail(hdev, &did, pdidd, cbRequired, &cbRequired, 0))
            goto endPowerSupply;

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
                NULL) ||
            !bqi.BatteryTag)
            goto endPowerSupply;

        BATTERY_INFORMATION bi = {0};
        bqi.InformationLevel = BatteryInformation;

        if (!DeviceIoControl(
                hBattery, IOCTL_BATTERY_QUERY_INFORMATION, &bqi, sizeof(bqi), &bi, sizeof(bi), &dwOut, NULL))
            goto endPowerSupply;

        if (!power_supply_root)
            netdata_allocate_power_supply(pdidd->DevicePath);

        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, sizeof(name), "BAT%d", i + 1);

        if (likely(power_supply_root->name))
            freez(power_supply_root->name);
        if (likely(power_supply_root->capacity->filename))
            freez(power_supply_root->capacity->filename);

        power_supply_root->name = power_supply_root->capacity->filename = NULL;
        power_supply_root->name = strdupz(name);
        power_supply_root->capacity->filename = strdupz(power_supply_root->name);

        netdata_update_power_supply_values(hBattery, &voltage, &bi, &bqi);

        netdata_power_supply_plot(&voltage, update_every);
    endPowerSupply:
        if (hBattery)
            CloseHandle(hBattery);

        if (pdidd)
            LocalFree(pdidd);
    }

    SetupDiDestroyDeviceInfoList(hdev);

    return 0;
}
