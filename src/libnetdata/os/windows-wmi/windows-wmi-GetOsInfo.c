// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi-GetOsInfo.h"

#if defined(OS_WINDOWS)

bool GetOsInfo(OsInfoWMI *osInfo) {
    if (!osInfo) return false;

    osInfo->Caption[0] = '\0';
    osInfo->ProductType = 0;

    if (InitializeWMI() != S_OK) return false;

    HRESULT hr;
    IEnumWbemClassObject *pEnumerator = NULL;

    BSTR query = SysAllocString(L"SELECT Caption, ProductType FROM Win32_OperatingSystem");
    BSTR wql = SysAllocString(L"WQL");
    hr = nd_wmi.pSvc->lpVtbl->ExecQuery(
        nd_wmi.pSvc,
        wql,
        query,
        WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
        NULL,
        &pEnumerator);
    SysFreeString(query);
    SysFreeString(wql);

    if (FAILED(hr)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "GetOsInfo() WMI query failed. Error code = 0x%X", hr);
        return false;
    }

    IWbemClassObject *pclsObj = NULL;
    ULONG uReturn = 0;
    bool success = false;

    hr = pEnumerator->lpVtbl->Next(pEnumerator, WBEM_INFINITE, 1, &pclsObj, &uReturn);
    if (uReturn > 0 && SUCCEEDED(hr)) {
        VARIANT vtProp;

        hr = pclsObj->lpVtbl->Get(pclsObj, L"Caption", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wcstombs(osInfo->Caption, vtProp.bstrVal, sizeof(osInfo->Caption) - 1);
            osInfo->Caption[sizeof(osInfo->Caption) - 1] = '\0';
        }
        VariantClear(&vtProp);

        hr = pclsObj->lpVtbl->Get(pclsObj, L"ProductType", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && (vtProp.vt == VT_I4 || vtProp.vt == VT_UI4))
            osInfo->ProductType = (DWORD)vtProp.uintVal;
        VariantClear(&vtProp);

        pclsObj->lpVtbl->Release(pclsObj);
        success = osInfo->Caption[0] != '\0';
    }

    pEnumerator->lpVtbl->Release(pEnumerator);
    return success;
}

#endif
