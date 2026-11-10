// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_WINDOWS_WMI_H
#define NETDATA_WINDOWS_WMI_H

#include "../../libnetdata.h"

#if defined(OS_WINDOWS)
typedef struct {
    IWbemLocator *pLoc;
    IWbemServices *pSvc;
    bool com_initialized;
    wchar_t namespace_path[128]; // namespace pSvc is connected to; empty when disconnected

    // Cancellation state, shared with the thread calling nd_wmi_cancel(); accessed atomically.
    DWORD connect_thread_id; // published before 'connecting' is set
    bool connecting;         // a ConnectServer() call is in flight
    bool cancelled;          // nd_wmi_cancel() was called; every later connect fails
} ND_WMI;

// Thread-local ROOT\CIMV2 connection used by InitializeWMI()/CleanupWMI().
extern __thread ND_WMI nd_wmi;

HRESULT InitializeWMI(void);
void CleanupWMI(void);

// Connect a caller-owned handle to any WMI namespace; must be used and disconnected on the same thread.
// Returns S_OK when already connected to the same namespace; a different namespace reconnects. On failure, *failed_step (optional) names the step that failed.
// ConnectServer() cannot be bounded below ~2 minutes (only cancelled, see nd_wmi_cancel()), so never call this
// from a shared collection loop.
HRESULT nd_wmi_connect(ND_WMI *wmi, const wchar_t *namespace_path, const char **failed_step);
void nd_wmi_disconnect(ND_WMI *wmi);

// From another thread: make every future nd_wmi_connect() on this handle fail and cancel an in-flight
// ConnectServer() with CoCancelCall(), retrying for up to max_wait_ut until it returns. The handle must outlive
// the call; the owning thread still disconnects it.
void nd_wmi_cancel(ND_WMI *wmi, usec_t max_wait_ut);

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
