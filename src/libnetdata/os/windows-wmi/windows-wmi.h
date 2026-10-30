// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_H
#define NETDATA_WINDOWS_WMI_H

#include "../../libnetdata.h"

#if defined(OS_WINDOWS)
typedef struct {
    IWbemLocator *pLoc;
    IWbemServices *pSvc;
    bool com_initialized;
} ND_WMI;

// Thread-local ROOT\CIMV2 connection used by InitializeWMI()/CleanupWMI().
extern __thread ND_WMI nd_wmi;

HRESULT InitializeWMI(void);
void CleanupWMI(void);

// Connect a caller-owned handle to any WMI namespace; must be used and disconnected on the same thread.
// Returns S_OK when already connected. On failure, *failed_step (optional) names the step that failed.
// ConnectServer() cannot be bounded below ~2 minutes, so never call this from a shared collection loop.
HRESULT nd_wmi_connect(ND_WMI *wmi, const wchar_t *namespace_path, const char **failed_step);
void nd_wmi_disconnect(ND_WMI *wmi);

// Run a forward-only, semi-synchronous WQL query; iterate with a bounded Next() timeout.
HRESULT nd_wmi_exec_query(IWbemServices *svc, const wchar_t *query_text, IEnumWbemClassObject **enumerator);

// Typed property getters; return false when the property is missing, NULL, or of an unexpected type.
bool nd_wmi_get_string_property(IWbemClassObject *obj, const wchar_t *prop, char *out, size_t out_size);
bool nd_wmi_get_uint64_property(IWbemClassObject *obj, const wchar_t *prop, uint64_t *out);
bool nd_wmi_get_bool_property(IWbemClassObject *obj, const wchar_t *prop, bool *out);

// Convert a WMI UTF-16 BSTR into a UTF-8, always null-terminated destination buffer.
// dst is emptied on a NULL src or on conversion failure. Safe with dst_size == 0.
void wmi_bstr_to_multibyte(char *dst, size_t dst_size, BSTR src);

#include "windows-wmi-GetDiskDriveInfo.h"
#include "windows-wmi-GetSystemInfo.h"

#endif

#endif //NETDATA_WINDOWS_WMI_H
