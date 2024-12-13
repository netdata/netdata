// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-wmi-GetMSSQLDataFileSize.h"

#if defined(OS_WINDOWS)

DICTIONARY *DatabaseSize = NULL;

void dict_mssql_insert_databases_size_cb(const DICTIONARY_ITEM *item __maybe_unused,
                                         void *value __maybe_unused,
                                         void *data __maybe_unused) {
}

size_t GetSQLDataFileSizeWMI()
{
    if (InitializeWMI() != S_OK)
        return 0;

    HRESULT hr;
    IEnumWbemClassObject *pEnumerator = NULL;

    if (!DatabaseSize) {
        DatabaseSize = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                  NULL,
                                                  sizeof(unsigned long long));

        dictionary_register_insert_callback(DatabaseSize, dict_mssql_insert_databases_size_cb, NULL);
    }

    // Execute the query, including new properties
    BSTR query = SysAllocString(L"SELECT Name, DataFilesSizeKB FROM Win32_PerfRawData_MSSQLSERVER_SQLServerDatabases WHERE Name <> '_Total'");
    BSTR wql = SysAllocString(L"WQL");
    hr = nd_wmi.pSvc->lpVtbl->ExecQuery(nd_wmi.pSvc,
                                        wql,
                                        query,
                                        WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
                                        NULL,
                                        &pEnumerator);

    if (FAILED(hr)) {
        nd_log(NDLS_COLLECTORS, NDLP_ERR, "GetSQLDataFileSizeWMI() WMI query failed. Error code = 0x%X", hr);
        return 0;
    }

    // Iterate through the results
    IWbemClassObject *pclsObj = NULL;
    ULONG uReturn = 0;
    size_t index = 0;
    char DatabaseName[NETDATA_MSSQL_MAX_DB_NAME + 1];
    while (pEnumerator) {
        hr = pEnumerator->lpVtbl->Next(pEnumerator, WBEM_INFINITE, 1, &pclsObj, &uReturn);
        if (0 == uReturn)
            break;

        VARIANT vtProp;

        // Extract DeviceID
        hr = pclsObj->lpVtbl->Get(pclsObj, L"Name", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR) {
            wcstombs(DatabaseName, vtProp.bstrVal, sizeof(DatabaseName));
        } else
            DatabaseName[0] = '\0';
        VariantClear(&vtProp);

        // Extract Model
        hr = pclsObj->lpVtbl->Get(pclsObj, L"DataFilesSizeKB", 0, &vtProp, 0, 0);
        if (SUCCEEDED(hr) && vtProp.vt == VT_BSTR && DatabaseName[0]) {
            char sizeStr[64];
            wcstombs(sizeStr, vtProp.bstrVal, sizeof(sizeStr));

            unsigned long long *value = dictionary_set(DatabaseSize, DatabaseName, NULL, sizeof(*value));
            if (!value)
                continue;

            *value = strtoull(sizeStr, NULL, 10);
        }
        VariantClear(&vtProp);

        pclsObj->lpVtbl->Release(pclsObj);
        index++;
    }

    pEnumerator->lpVtbl->Release(pEnumerator);

    return index;
}

#endif
