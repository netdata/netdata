// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi-GetSystemInfo.h"

#if defined(OS_WINDOWS)

static bool wmi_get_string_property(IWbemClassObject *pclsObj, const wchar_t *prop, char *out, size_t out_size) {
    if(!pclsObj || !out || out_size == 0) return false;

    VARIANT vtProp;
    HRESULT hr = pclsObj->lpVtbl->Get(pclsObj, prop, 0, &vtProp, 0, 0);
    if(FAILED(hr) || vtProp.vt != VT_BSTR) {
        VariantClear(&vtProp);
        return false;
    }

    wcstombs(out, vtProp.bstrVal, out_size - 1);
    out[out_size - 1] = '\0';
    VariantClear(&vtProp);
    return true;
}

bool GetWin32ComputerSystemInfo(Win32ComputerSystemInfo *out) {
    if(!out) return false;
    memset(out, 0, sizeof(*out));

    if(InitializeWMI() != S_OK) return false;

    BSTR query = SysAllocString(L"SELECT Model, Manufacturer FROM Win32_ComputerSystem");
    BSTR wql = SysAllocString(L"WQL");

    IEnumWbemClassObject *pEnumerator = NULL;
    HRESULT hr = nd_wmi.pSvc->lpVtbl->ExecQuery(
        nd_wmi.pSvc, wql, query,
        WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
        NULL, &pEnumerator);

    SysFreeString(query);
    SysFreeString(wql);

    if(FAILED(hr) || !pEnumerator) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "GetWin32ComputerSystemInfo() WMI query failed. Error code = 0x%X", hr);
        if(pEnumerator) pEnumerator->lpVtbl->Release(pEnumerator);
        return false;
    }

    IWbemClassObject *pclsObj = NULL;
    ULONG uReturn = 0;
    bool found = false;
    hr = pEnumerator->lpVtbl->Next(pEnumerator, WBEM_INFINITE, 1, &pclsObj, &uReturn);
    if(uReturn > 0 && pclsObj) {
        wmi_get_string_property(pclsObj, L"Model", out->Model, sizeof(out->Model));
        wmi_get_string_property(pclsObj, L"Manufacturer", out->Manufacturer, sizeof(out->Manufacturer));
        out->Populated = true;
        found = true;
        pclsObj->lpVtbl->Release(pclsObj);
    }

    pEnumerator->lpVtbl->Release(pEnumerator);
    return found;
}

bool GetWin32OperatingSystemInfo(Win32OperatingSystemInfo *out) {
    if(!out) return false;
    memset(out, 0, sizeof(*out));

    if(InitializeWMI() != S_OK) return false;

    BSTR query = SysAllocString(L"SELECT Caption FROM Win32_OperatingSystem");
    BSTR wql = SysAllocString(L"WQL");

    IEnumWbemClassObject *pEnumerator = NULL;
    HRESULT hr = nd_wmi.pSvc->lpVtbl->ExecQuery(
        nd_wmi.pSvc, wql, query,
        WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
        NULL, &pEnumerator);

    SysFreeString(query);
    SysFreeString(wql);

    if(FAILED(hr) || !pEnumerator) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "GetWin32OperatingSystemInfo() WMI query failed. Error code = 0x%X", hr);
        if(pEnumerator) pEnumerator->lpVtbl->Release(pEnumerator);
        return false;
    }

    IWbemClassObject *pclsObj = NULL;
    ULONG uReturn = 0;
    bool found = false;
    hr = pEnumerator->lpVtbl->Next(pEnumerator, WBEM_INFINITE, 1, &pclsObj, &uReturn);
    if(uReturn > 0 && pclsObj) {
        wmi_get_string_property(pclsObj, L"Caption", out->Caption, sizeof(out->Caption));
        out->Populated = true;
        found = true;
        pclsObj->lpVtbl->Release(pclsObj);
    }

    pEnumerator->lpVtbl->Release(pEnumerator);
    return found;
}

#endif