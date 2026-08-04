// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi.h"
#include "windows-wmi-GetSystemInfo.h"
#include "libnetdata/environment/environment.h"

#if defined(OS_WINDOWS)

#define WMI_GETSYSTEMINFO_TIMEOUT_DEFAULT_MS 5000

static ULONG wmi_getsysteminfo_timeout_ms(void) {
    CLEAN_CHAR_P *env = nd_environment_get_dup("NETDATA_WMI_STARTUP_TIMEOUT_MS");
    if(!env || !*env) return WMI_GETSYSTEMINFO_TIMEOUT_DEFAULT_MS;

    char *end = NULL;
    long v = strtol(env, &end, 10);
    if(!end || *end || end == env || v < 100 || v > 60000)
        return WMI_GETSYSTEMINFO_TIMEOUT_DEFAULT_MS;

    return (ULONG)v;
}

static bool wmi_get_string_property(IWbemClassObject *pclsObj, const wchar_t *prop, char *out, size_t out_size) {
    if(!pclsObj || !out || out_size == 0) return false;

    VARIANT vtProp;
    VariantInit(&vtProp);
    HRESULT hr = pclsObj->lpVtbl->Get(pclsObj, prop, 0, &vtProp, 0, 0);
    if(FAILED(hr) || vtProp.vt != VT_BSTR) {
        VariantClear(&vtProp);
        return false;
    }

    // Shared helper converts the UTF-16 BSTR to UTF-8 (locale-independent) and always
    // null-terminates; empties out on conversion failure.
    wmi_bstr_to_multibyte(out, out_size, vtProp.bstrVal);
    VariantClear(&vtProp);
    return true;
}

static IWbemClassObject *wmi_exec_single_row_query(const wchar_t *query_text, const char *caller) {
    HRESULT init_hr = InitializeWMI();
    if(FAILED(init_hr) || !nd_wmi.pSvc)
        return NULL;

    BSTR query = SysAllocString(query_text);
    BSTR wql = SysAllocString(L"WQL");
    if(!query || !wql) {
        // SysAllocString() returns NULL only on allocation failure here; do not
        // hand a NULL BSTR to ExecQuery(). SysFreeString(NULL) is a safe no-op.
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "%s WMI query aborted: SysAllocString() failed (out of memory)", caller);
        SysFreeString(query);
        SysFreeString(wql);
        return NULL;
    }

    IEnumWbemClassObject *pEnumerator = NULL;
    HRESULT hr = nd_wmi.pSvc->lpVtbl->ExecQuery(
        nd_wmi.pSvc, wql, query,
        WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
        NULL, &pEnumerator);

    SysFreeString(query);
    SysFreeString(wql);

    if(FAILED(hr) || !pEnumerator) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "%s WMI query failed. Error code = 0x%X", caller, hr);
        if(pEnumerator)
            pEnumerator->lpVtbl->Release(pEnumerator);
        return NULL;
    }

    ULONG timeout_ms = wmi_getsysteminfo_timeout_ms();

    IWbemClassObject *pclsObj = NULL;
    ULONG uReturn = 0;
    hr = pEnumerator->lpVtbl->Next(pEnumerator, timeout_ms, 1, &pclsObj, &uReturn);
    pEnumerator->lpVtbl->Release(pEnumerator);

    if(FAILED(hr) || uReturn == 0 || !pclsObj) {
        if(hr == WBEM_S_TIMEDOUT && uReturn == 0) {
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "%s WMI query timed out after %lu ms", caller, timeout_ms);
        }
        else if(!FAILED(hr) && uReturn == 0) {
            nd_log(NDLS_DAEMON, NDLP_DEBUG,
                   "%s WMI query returned no rows", caller);
        }
        return NULL;
    }

    return pclsObj;
}

bool GetWin32ComputerSystemInfo(Win32ComputerSystemInfo *out) {
    if(!out) return false;
    memset(out, 0, sizeof(*out));

    IWbemClassObject *pclsObj = wmi_exec_single_row_query(
        L"SELECT Model, Manufacturer FROM Win32_ComputerSystem",
        "GetWin32ComputerSystemInfo()");
    if(!pclsObj)
        return false;

    wmi_get_string_property(pclsObj, L"Model", out->Model, sizeof(out->Model));
    wmi_get_string_property(pclsObj, L"Manufacturer", out->Manufacturer, sizeof(out->Manufacturer));
    out->Populated = true;
    pclsObj->lpVtbl->Release(pclsObj);
    return true;
}

#endif
