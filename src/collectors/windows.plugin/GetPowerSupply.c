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

struct win_battery {
    struct power_supply ps;
    struct simple_property voltage;
    bool seen; // true if discovered in the current collection pass
    struct win_battery *next;
};

static struct win_battery *batteries_root = NULL;

static struct win_battery *netdata_get_or_create_battery(const char *name)
{
    for (struct win_battery *battery = batteries_root; battery; battery = battery->next) {
        if (battery->ps.name && !strcmp(battery->ps.name, name))
            return battery;
    }

    struct win_battery *battery = callocz(1, sizeof(*battery));
    battery->ps.name = strdupz(name);
    battery->ps.capacity = callocz(1, sizeof(struct simple_property));
    battery->next = batteries_root;
    batteries_root = battery;

    return battery;
}

static inline void netdata_update_power_supply_values(
    struct win_battery *battery,
    HANDLE hBattery,
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
        num = (den) ? num / den : 0;

        battery->ps.capacity->value = (unsigned long long)(num * 100.0);
    }

    if (bs.Voltage != BATTERY_UNKNOWN_VOLTAGE)
        battery->voltage.value = bs.Voltage;
}

static void netdata_power_supply_plot(struct win_battery *battery, int update_every)
{
    rrdset_create_simple_prop(
        &battery->ps,
        battery->ps.capacity,
        "Battery capacity",
        "capacity",
        1,
        "percentage",
        NETDATA_CHART_PRIO_POWER_SUPPLY_CAPACITY,
        update_every);

    rrdset_create_simple_prop(
        &battery->ps,
        &battery->voltage,
        "Power supply voltage",
        "now",
        1000,
        "v",
        NETDATA_CHART_PRIO_POWER_SUPPLY_VOLTAGE,
        update_every);
}

void do_GetPowerSupply_cleanup(void)
{
    while (batteries_root) {
        struct win_battery *battery = batteries_root;
        batteries_root = battery->next;

        if (battery->ps.capacity && battery->ps.capacity->st)
            rrdset_is_obsolete___safe_from_collector_thread(battery->ps.capacity->st);
        if (battery->voltage.st)
            rrdset_is_obsolete___safe_from_collector_thread(battery->voltage.st);

        freez(battery->ps.name);
        freez(battery->ps.capacity);
        freez(battery);
    }
}

int do_GetPowerSupply(int update_every, usec_t dt __maybe_unused)
{
    for (struct win_battery *b = batteries_root; b; b = b->next)
        b->seen = false;

    HDEVINFO hdev = SetupDiGetClassDevs(&GUID_DEVCLASS_BATTERY, 0, 0, DIGCF_PRESENT | DIGCF_DEVICEINTERFACE);
    if (hdev == INVALID_HANDLE_VALUE)
        return -1;

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

        char name[RRD_ID_LENGTH_MAX + 1];
        snprintfz(name, sizeof(name), "BAT%d", i + 1);
        struct win_battery *battery = netdata_get_or_create_battery(name);
        battery->seen = true;
        netdata_update_power_supply_values(battery, hBattery, &bi, &bqi);
        netdata_power_supply_plot(battery, update_every);

    endPowerSupply:
        if (hBattery != NULL && hBattery != INVALID_HANDLE_VALUE)
            CloseHandle(hBattery);

        if (pdidd)
            LocalFree(pdidd);
    }

    SetupDiDestroyDeviceInfoList(hdev);

    // Retire batteries that were not seen in this discovery pass (e.g. physically removed).
    struct win_battery **pp = &batteries_root;
    while (*pp) {
        struct win_battery *b = *pp;
        if (!b->seen) {
            if (b->ps.capacity && b->ps.capacity->st)
                rrdset_is_obsolete___safe_from_collector_thread(b->ps.capacity->st);
            if (b->voltage.st)
                rrdset_is_obsolete___safe_from_collector_thread(b->voltage.st);
            *pp = b->next;
            freez(b->ps.name);
            freez(b->ps.capacity);
            freez(b);
        } else {
            pp = &b->next;
        }
    }

    return 0;
}
