// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi.h"

#if defined(OS_WINDOWS)

__thread ND_WMI nd_wmi = { 0 };

HRESULT InitializeWMI(void) {
    if(nd_wmi.pLoc && nd_wmi.pSvc) return S_OK;
    CleanupWMI();

    IWbemLocator **pLoc = &nd_wmi.pLoc;
    IWbemServices **pSvc = &nd_wmi.pSvc;

    HRESULT hr;

    // Initialize COM
    hr = CoInitializeEx(0, COINIT_MULTITHREADED);
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Failed to initialize COM library. Error code = 0x%X", hr);
        CleanupWMI();
        return hr;
    }

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
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Failed to initialize security. Error code = 0x%X", hr);
        CleanupWMI();
        return hr;
    }

    // Obtain the initial locator to WMI
    hr = CoCreateInstance(
            &CLSID_WbemLocator, 0,
            CLSCTX_INPROC_SERVER,
            &IID_IWbemLocator, (LPVOID *)pLoc
    );
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Failed to create IWbemLocator object. Error code = 0x%X", hr);
        CleanupWMI();
        return hr;
    }

    // Connect to WMI
    BSTR namespacePath = SysAllocString(L"ROOT\\CIMV2");
    hr = (*pLoc)->lpVtbl->ConnectServer(
            *pLoc,
            namespacePath,
            NULL,
            NULL,
            0,
            0,
            0,
            0,
            pSvc
    );
    SysFreeString(namespacePath);

    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Could not connect to WMI server. Error code = 0x%X", hr);
        CleanupWMI();
        return hr;
    }

    // Set security levels on the proxy
    hr = CoSetProxyBlanket(
            (IUnknown *)*pSvc,
            RPC_C_AUTHN_WINNT,
            RPC_C_AUTHZ_NONE,
            NULL,
            RPC_C_AUTHN_LEVEL_CALL,
            RPC_C_IMP_LEVEL_IMPERSONATE,
            NULL,
            EOAC_NONE
    );
    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "Could not set proxy blanket. Error code = 0x%X", hr);
        CleanupWMI();
        return hr;
    }

    return S_OK;
}

void CleanupWMI(void) {
    if(nd_wmi.pLoc)
        nd_wmi.pLoc->lpVtbl->Release(nd_wmi.pLoc);

    if (nd_wmi.pSvc)
        nd_wmi.pSvc->lpVtbl->Release(nd_wmi.pSvc);

    nd_wmi.pLoc = NULL;
    nd_wmi.pSvc = NULL;

    CoUninitialize();
}

#endif