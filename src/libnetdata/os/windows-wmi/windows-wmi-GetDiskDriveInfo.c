// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi-GetDiskDriveInfo.h"

#if defined(OS_WINDOWS)

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
            wcstombs(diskInfoArray[index].DeviceID, vtProp.bstrVal, sizeof(diskInfoArray[index].DeviceID));
        }
        VariantClear(&vtProp);

        // Extract Model
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Model", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wcstombs(diskInfoArray[index].Model, vtProp.bstrVal, sizeof(diskInfoArray[index].Model));
        }
        VariantClear(&vtProp);

        // Extract Caption
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Caption", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wcstombs(diskInfoArray[index].Caption, vtProp.bstrVal, sizeof(diskInfoArray[index].Caption));
        }
        VariantClear(&vtProp);

        // Extract Name
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Name", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wcstombs(diskInfoArray[index].Name, vtProp.bstrVal, sizeof(diskInfoArray[index].Name));
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
            wcstombs(sizeStr, vtProp.bstrVal, sizeof(sizeStr));
            diskInfoArray[index].Size = strtoull(sizeStr, NULL, 10);
        }
        VariantClear(&vtProp);

        // Extract Status
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Status", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wcstombs(diskInfoArray[index].Status, vtProp.bstrVal, sizeof(diskInfoArray[index].Status));
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
            wcstombs(diskInfoArray[index].Manufacturer, vtProp.bstrVal, sizeof(diskInfoArray[index].Manufacturer));
        }
        VariantClear(&vtProp);

        // Extract InstallDate
        hr = pclsObj->lpVtbl->Get(pclsObj, L"InstallDate", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wcstombs(diskInfoArray[index].InstallDate, vtProp.bstrVal, sizeof(diskInfoArray[index].InstallDate));
        }
        VariantClear(&vtProp);

        // Extract MediaType
        hr = pclsObj->lpVtbl->Get(pclsObj, L"MediaType", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wcstombs(diskInfoArray[index].MediaType, vtProp.bstrVal, sizeof(diskInfoArray[index].MediaType));
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