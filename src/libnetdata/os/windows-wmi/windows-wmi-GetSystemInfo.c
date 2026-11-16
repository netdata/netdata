// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi.h"
#include "windows-wmi-GetSystemInfo.h"

#if defined(OS_WINDOWS)

#define WMI_GETSYSTEMINFO_TIMEOUT_DEFAULT_MS 5000

static ULONG wmi_getsysteminfo_timeout_ms(void) {
    const char *env = getenv("NETDATA_WMI_STARTUP_TIMEOUT_MS");
    if(!env || !*env) return WMI_GETSYSTEMINFO_TIMEOUT_DEFAULT_MS;

    char *end = NULL;
    long v = strtol(env, &end, 10);
    if(!end || *end || end == env || v < 100 || v > 60000)
        return WMI_GETSYSTEMINFO_TIMEOUT_DEFAULT_MS;

    return (ULONG)v;
}

static IWbemClassObject *wmi_exec_single_row_query(const wchar_t *query_text, const char *caller) {
    HRESULT init_hr = InitializeWMI();
    if(FAILED(init_hr) || !nd_wmi.pSvc)
        return NULL;

    IEnumWbemClassObject *pEnumerator = NULL;
    HRESULT hr = nd_wmi_exec_query(nd_wmi.pSvc, query_text, &pEnumerator);
    if(FAILED(hr)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "%s WMI query failed. Error code = 0x%X", caller, hr);
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

    nd_wmi_get_string_property(pclsObj, L"Model", out->Model, sizeof(out->Model));
    nd_wmi_get_string_property(pclsObj, L"Manufacturer", out->Manufacturer, sizeof(out->Manufacturer));
    out->Populated = true;
    pclsObj->lpVtbl->Release(pclsObj);
    return true;
}

#endif
