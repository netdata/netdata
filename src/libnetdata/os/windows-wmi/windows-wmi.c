// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi.h"

#if defined(OS_WINDOWS)

__thread ND_WMI nd_wmi = { 0 };

void wmi_bstr_to_multibyte(char *dst, size_t dst_size, BSTR src)
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

HRESULT nd_wmi_connect(ND_WMI *wmi, const wchar_t *namespace_path, const char **failed_step) {
    const char *step_dummy;
    if (!failed_step)
        failed_step = &step_dummy;
    *failed_step = NULL;

    if(wmi->pLoc && wmi->pSvc) return S_OK;
    nd_wmi_disconnect(wmi);

    HRESULT hr;

    // Initialize COM
    hr = CoInitializeEx(0, COINIT_MULTITHREADED);
    if (FAILED(hr)) {
        *failed_step = "Failed to initialize COM library";
        return hr;
    }
    // S_FALSE (already initialized) also needs a matching CoUninitialize().
    wmi->com_initialized = true;

    // Set COM security levels
    hr = CoInitializeSecurity(
            NULL,
            -1,
            NULL,
            NULL,
            RPC_C_AUTHN_LEVEL_DEFAULT,
            RPC_C_IMP_LEVEL_IMPERSONATE,
            NULL,
            EOAC_NONE,
            NULL
    );
    if (FAILED(hr) && hr != RPC_E_TOO_LATE) {
        *failed_step = "Failed to initialize security";
        nd_wmi_disconnect(wmi);
        return hr;
    }

    // Obtain the initial locator to WMI
    hr = CoCreateInstance(
            &CLSID_WbemLocator, 0,
            CLSCTX_INPROC_SERVER,
            &IID_IWbemLocator, (LPVOID *)&wmi->pLoc
    );
    if (FAILED(hr)) {
        *failed_step = "Failed to create IWbemLocator object";
        nd_wmi_disconnect(wmi);
        return hr;
    }

    // Connect to WMI.
    // ConnectServer() has no custom-timeout knob; WBEM_FLAG_CONNECT_USE_MAX_WAIT is the only mechanism and caps
    // the connect at ~2 minutes, turning an indefinite hang on an unhealthy WMI service into a bounded delay.
    BSTR namespacePath = SysAllocString(namespace_path);
    if (!namespacePath) {
        *failed_step = "Failed to allocate WMI namespace path";
        nd_wmi_disconnect(wmi);
        return E_OUTOFMEMORY;
    }
    hr = wmi->pLoc->lpVtbl->ConnectServer(
            wmi->pLoc,
            namespacePath,
            NULL,
            NULL,
            0,
            WBEM_FLAG_CONNECT_USE_MAX_WAIT,
            0,
            0,
            &wmi->pSvc
    );
    SysFreeString(namespacePath);

    if (FAILED(hr)) {
        *failed_step = "Could not connect to WMI server";
        nd_wmi_disconnect(wmi);
        return hr;
    }

    // Set security levels on the proxy
    hr = CoSetProxyBlanket(
            (IUnknown *)wmi->pSvc,
            RPC_C_AUTHN_WINNT,
            RPC_C_AUTHZ_NONE,
            NULL,
            RPC_C_AUTHN_LEVEL_CALL,
            RPC_C_IMP_LEVEL_IMPERSONATE,
            NULL,
            EOAC_NONE
    );
    if (FAILED(hr)) {
        *failed_step = "Could not set proxy blanket";
        nd_wmi_disconnect(wmi);
        return hr;
    }

    return S_OK;
}

void nd_wmi_disconnect(ND_WMI *wmi) {
    if (wmi->pSvc)
        wmi->pSvc->lpVtbl->Release(wmi->pSvc);

    if(wmi->pLoc)
        wmi->pLoc->lpVtbl->Release(wmi->pLoc);

    wmi->pLoc = NULL;
    wmi->pSvc = NULL;

    // Uninitialize only what this handle initialized, keeping the per-thread COM reference count balanced.
    if (wmi->com_initialized) {
        CoUninitialize();
        wmi->com_initialized = false;
    }
}

HRESULT InitializeWMI(void) {
    // These probes run synchronously on the daemon startup path (before rrd_init()). The query iteration is
    // time-bounded (Next() in the GetSystemInfo helper); the connect is bounded by nd_wmi_connect().
    const char *failed_step = NULL;
    HRESULT hr = nd_wmi_connect(&nd_wmi, L"ROOT\\CIMV2", &failed_step);
    if (FAILED(hr))
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "%s. Error code = 0x%X", failed_step ? failed_step : "WMI connect failed", hr);

    return hr;
}

void CleanupWMI(void) {
    nd_wmi_disconnect(&nd_wmi);
}

HRESULT nd_wmi_exec_query(IWbemServices *svc, const wchar_t *query_text, IEnumWbemClassObject **enumerator) {
    *enumerator = NULL;
    if (!svc)
        return E_POINTER;

    BSTR query = SysAllocString(query_text);
    BSTR wql = SysAllocString(L"WQL");
    if(!query || !wql) {
        // Do not hand a NULL BSTR to ExecQuery(); SysFreeString(NULL) is a safe no-op.
        SysFreeString(query);
        SysFreeString(wql);
        return E_OUTOFMEMORY;
    }

    HRESULT hr = svc->lpVtbl->ExecQuery(
        svc, wql, query,
        WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
        NULL, enumerator);

    SysFreeString(query);
    SysFreeString(wql);

    if (FAILED(hr) || !*enumerator) {
        if (*enumerator)
            (*enumerator)->lpVtbl->Release(*enumerator);
        *enumerator = NULL;
        return FAILED(hr) ? hr : E_FAIL;
    }

    return S_OK;
}

bool nd_wmi_get_string_property(IWbemClassObject *obj, const wchar_t *prop, char *out, size_t out_size) {
    if(!obj || !out || out_size == 0) return false;

    VARIANT vtProp;
    VariantInit(&vtProp);
    HRESULT hr = obj->lpVtbl->Get(obj, prop, 0, &vtProp, 0, 0);
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

bool nd_wmi_get_uint64_property(IWbemClassObject *obj, const wchar_t *prop, uint64_t *out) {
    if (!obj || !out) return false;

    VARIANT vtProp;
    VariantInit(&vtProp);
    HRESULT hr = obj->lpVtbl->Get(obj, prop, 0, &vtProp, 0, 0);
    if (FAILED(hr)) {
        VariantClear(&vtProp);
        return false;
    }

    bool ok = true;
    switch (vtProp.vt) {
        // WMI marshals CIM uint64/sint64 as decimal strings.
        case VT_BSTR: {
            char buf[32];
            char *end = NULL;
            wmi_bstr_to_multibyte(buf, sizeof(buf), vtProp.bstrVal);
            errno_clear();
            unsigned long long v = strtoull(buf, &end, 10);
            if (!buf[0] || !end || *end || errno == ERANGE || buf[0] == '-')
                ok = false;
            else
                *out = (uint64_t)v;
            break;
        }
        case VT_UI8: *out = (uint64_t)vtProp.ullVal; break;
        case VT_I8:  ok = vtProp.llVal >= 0; if (ok) *out = (uint64_t)vtProp.llVal; break;
        case VT_UI4: *out = (uint64_t)vtProp.ulVal; break;
        case VT_I4:  ok = vtProp.lVal >= 0; if (ok) *out = (uint64_t)vtProp.lVal; break;
        default: ok = false; break;
    }

    VariantClear(&vtProp);
    return ok;
}

bool nd_wmi_get_bool_property(IWbemClassObject *obj, const wchar_t *prop, bool *out) {
    if (!obj || !out) return false;

    VARIANT vtProp;
    VariantInit(&vtProp);
    HRESULT hr = obj->lpVtbl->Get(obj, prop, 0, &vtProp, 0, 0);
    bool ok = SUCCEEDED(hr) && vtProp.vt == VT_BOOL;
    if (ok)
        *out = vtProp.boolVal != VARIANT_FALSE;

    VariantClear(&vtProp);
    return ok;
}

#endif
