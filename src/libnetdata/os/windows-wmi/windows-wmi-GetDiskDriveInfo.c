// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi.h"
#include "windows-wmi-GetDiskDriveInfo.h"

#if defined(OS_WINDOWS)

// Total time allowed for enumerating Win32_DiskDrive, across all Next() calls.
#define WMI_DISKDRIVE_ENUM_TIMEOUT_MS 5000

size_t GetDiskDriveInfo(DiskDriveInfoWMI *diskInfoArray, size_t array_size) {
    HRESULT init_hr = InitializeWMI();
    if (FAILED(init_hr) || !nd_wmi.pSvc) return 0;

    IEnumWbemClassObject *pEnumerator = NULL;
    HRESULT hr = nd_wmi_exec_query(nd_wmi.pSvc, L"SELECT Index, Model, DeviceID FROM Win32_DiskDrive", &pEnumerator);
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "GetDiskDriveInfo() WMI query failed. Error code = 0x%X", hr);
        return 0;
    }

    usec_t deadline_ut = now_monotonic_usec() + WMI_DISKDRIVE_ENUM_TIMEOUT_MS * USEC_PER_MS;
    size_t index = 0;
    while (index < array_size) {
        usec_t now_ut = now_monotonic_usec();
        if (now_ut >= deadline_ut) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "GetDiskDriveInfo() WMI enumeration timed out after %d ms; using %zu disk(s)",
                   WMI_DISKDRIVE_ENUM_TIMEOUT_MS, index);
            break;
        }
        long remaining_ms = (long)((deadline_ut - now_ut + USEC_PER_MS - 1) / USEC_PER_MS);

        IWbemClassObject *pclsObj = NULL;
        ULONG uReturn = 0;
        hr = pEnumerator->lpVtbl->Next(pEnumerator, remaining_ms, 1, &pclsObj, &uReturn);
        if (FAILED(hr)) {
            nd_log(NDLS_COLLECTORS, NDLP_ERR,
                   "GetDiskDriveInfo() WMI enumeration failed. Error code = 0x%X; using %zu disk(s)", hr, index);
            break;
        }
        if (0 == uReturn || !pclsObj) {
            if (hr == WBEM_S_TIMEDOUT)
                continue;
            break;
        }

        DiskDriveInfoWMI *info = &diskInfoArray[index];

        uint64_t disk_index;
        if (nd_wmi_get_uint64_property(pclsObj, L"Index", &disk_index) && disk_index <= INT_MAX)
            info->Index = (int)disk_index;

        nd_wmi_get_string_property(pclsObj, L"Model", info->Model, sizeof(info->Model));
        nd_wmi_get_string_property(pclsObj, L"DeviceID", info->DeviceID, sizeof(info->DeviceID));

        pclsObj->lpVtbl->Release(pclsObj);
        index++;
    }

    pEnumerator->lpVtbl->Release(pEnumerator);

    return index;
}

#endif
