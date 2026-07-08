// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi-GetDiskDriveInfo.h"

#if defined(OS_WINDOWS)

static void wmi_bstr_to_multibyte(char *dst, size_t dst_size, BSTR src)
{
    if (!dst_size)
        return;

    dst[0] = '\0';

    if (!src)
        return;

    if (!utf16_to_utf8(dst, dst_size, src, -1, NULL)) {
        dst[0] = '\0';
        return;
    }
}

size_t GetDiskDriveInfo(DiskDriveInfoWMI *diskInfoArray, size_t array_size) {
    if (InitializeWMI() != S_OK) return 0;

    HRESULT hr;
    IEnumWbemClassObject* pEnumerator = NULL;

    // Execute the query, including new properties
    BSTR query = SysAllocString(L"SELECT DeviceID, Model, Caption, Name, Partitions, Size, Status, Availability, Index, Manufacturer, InstallDate, MediaType, NeedsCleaning FROM WIN32_DiskDrive");
    BSTR wql = SysAllocString(L"WQL");
    hr = nd_wmi.pSvc->lpVtbl->ExecQuery(
            nd_wmi.pSvc,
            wql,
            query,
            WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
            NULL,
            &pEnumerator
    );
    SysFreeString(query);
    SysFreeString(wql);

    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "GetDiskDriveInfo() WMI query failed. Error code = 0x%X", hr);
        return 0;
    }

    // Iterate through the results
    IWbemClassObject *pclsObj = NULL;
    ULONG uReturn = 0;
    size_t index = 0;
    while (pEnumerator && index < array_size) {
        hr = pEnumerator->lpVtbl->Next(pEnumerator, WBEM_INFINITE, 1, &pclsObj, &uReturn);
        if (0 == uReturn) break;

        VARIANT vtProp;

        // Extract DeviceID
        hr = pclsObj->lpVtbl->Get(pclsObj, L"DeviceID", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wmi_bstr_to_multibyte(
                diskInfoArray[index].DeviceID,
                sizeof(diskInfoArray[index].DeviceID),
                vtProp.bstrVal);
        }
        VariantClear(&vtProp);

        // Extract Model
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Model", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wmi_bstr_to_multibyte(
                diskInfoArray[index].Model,
                sizeof(diskInfoArray[index].Model),
                vtProp.bstrVal);
        }
        VariantClear(&vtProp);

        // Extract Caption
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Caption", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wmi_bstr_to_multibyte(
                diskInfoArray[index].Caption,
                sizeof(diskInfoArray[index].Caption),
                vtProp.bstrVal);
        }
        VariantClear(&vtProp);

        // Extract Name
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Name", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wmi_bstr_to_multibyte(
                diskInfoArray[index].Name,
                sizeof(diskInfoArray[index].Name),
                vtProp.bstrVal);
        }
        VariantClear(&vtProp);

        // Extract Partitions
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Partitions", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && (vtProp.vt == VT_I4 || vtProp.vt == VT_UI4)) {
            diskInfoArray[index].Partitions = vtProp.intVal;
        }
        VariantClear(&vtProp);

        // Extract Size (convert BSTR to uint64)
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Size", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            char sizeStr[64];
            wmi_bstr_to_multibyte(sizeStr, sizeof(sizeStr), vtProp.bstrVal);
            diskInfoArray[index].Size = strtoull(sizeStr, NULL, 10);
        }
        VariantClear(&vtProp);

        // Extract Status
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Status", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wmi_bstr_to_multibyte(
                diskInfoArray[index].Status,
                sizeof(diskInfoArray[index].Status),
                vtProp.bstrVal);
        }
        VariantClear(&vtProp);

        // Extract Availability
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Availability", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && (vtProp.vt == VT_I4 || vtProp.vt == VT_UI4)) {
            diskInfoArray[index].Availability = vtProp.intVal;
        }
        VariantClear(&vtProp);

        // Extract Index
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Index", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && (vtProp.vt == VT_I4 || vtProp.vt == VT_UI4)) {
            diskInfoArray[index].Index = vtProp.intVal;
        }
        VariantClear(&vtProp);

        // Extract Manufacturer
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Manufacturer", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wmi_bstr_to_multibyte(
                diskInfoArray[index].Manufacturer,
                sizeof(diskInfoArray[index].Manufacturer),
                vtProp.bstrVal);
        }
        VariantClear(&vtProp);

        // Extract InstallDate
        hr = pclsObj->lpVtbl->Get(pclsObj, L"InstallDate", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wmi_bstr_to_multibyte(
                diskInfoArray[index].InstallDate,
                sizeof(diskInfoArray[index].InstallDate),
                vtProp.bstrVal);
        }
        VariantClear(&vtProp);

        // Extract MediaType
        hr = pclsObj->lpVtbl->Get(pclsObj, L"MediaType", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wmi_bstr_to_multibyte(
                diskInfoArray[index].MediaType,
                sizeof(diskInfoArray[index].MediaType),
                vtProp.bstrVal);
        }
        VariantClear(&vtProp);

        // Extract NeedsCleaning
        hr = pclsObj->lpVtbl->Get(pclsObj, L"NeedsCleaning", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && (vtProp.vt == VT_BOOL)) {
            diskInfoArray[index].NeedsCleaning = vtProp.boolVal;
        }
        VariantClear(&vtProp);

        pclsObj->lpVtbl->Release(pclsObj);
        index++;
    }

    pEnumerator->lpVtbl->Release(pEnumerator);

    return index;
}

#endif