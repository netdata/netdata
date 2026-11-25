// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi.h"
#include "windows-wmi-GetDiskDriveInfo.h"

#if defined(OS_WINDOWS)

size_t GetDiskDriveInfo(DiskDriveInfoWMI *diskInfoArray, size_t array_size) {
    HRESULT init_hr = InitializeWMI();
    if (FAILED(init_hr) || !nd_wmi.pSvc) return 0;

    IEnumWbemClassObject *pEnumerator = NULL;
    HRESULT hr = nd_wmi_exec_query(nd_wmi.pSvc, L"SELECT Index, Model, DeviceID FROM Win32_DiskDrive", &pEnumerator);
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "GetDiskDriveInfo() WMI query failed. Error code = 0x%X", hr);
        return 0;
    }

    IWbemClassObject *pclsObj = NULL;
    ULONG uReturn = 0;
    size_t index = 0;
    while (index < array_size) {
        hr = pEnumerator->lpVtbl->Next(pEnumerator, WBEM_INFINITE, 1, &pclsObj, &uReturn);
        if (0 == uReturn) break;

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
