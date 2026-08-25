// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef _NETDATA_WIN_SYSTEM_INFO_H_
#define _NETDATA_WIN_SYSTEM_INFO_H_

// the netdata database
#include "database/rrd.h"

#define NETDATA_DEFAULT_SYSTEM_INFO_VALUE_UNKNOWN "unknown"
#define NETDATA_DEFAULT_SYSTEM_INFO_VALUE_NONE "none"
#define NETDATA_DEFAULT_SYSTEM_INFO_VALUE_FALSE "false"
#define NETDATA_WIN_DETECTION_METHOD "windows-api"

#define NETDATA_WIN_VIRT_KVM              "kvm"
#define NETDATA_WIN_VIRT_QEMU             "qemu"
#define NETDATA_WIN_VIRT_MICROSOFT        "microsoft"
#define NETDATA_WIN_VIRT_VMWARE           "vmware"
#define NETDATA_WIN_VIRT_ORACLE           "oracle"
#define NETDATA_WIN_VIRT_XEN              "xen"
#define NETDATA_WIN_VIRT_AMAZON           "amazon"
#define NETDATA_WIN_VIRT_PARALLELS        "parallels"
#define NETDATA_WIN_VIRT_DIGITALOCEAN     "digitalocean"
#define NETDATA_WIN_VIRT_BARE_METAL       "none"

// OS label prefixes. Centralised so the strlen()-of-literal pattern (which
// SonarCloud flags as S5837 / S5996) collapses to sizeof()-1 below.
#define NETDATA_WINDOWS_MICROSOFT_PREFIX       "Microsoft "
#define NETDATA_WINDOWS_OS_PREFIX_SERVER       "Windows Server"
#define NETDATA_WINDOWS_OS_PREFIX_CLIENT       "Windows"
#define NETDATA_WINDOWS_OS_PREFIX_SERVER_NAME  "Windows Server "

#define NETDATA_WIN_CONTAINER_NONE                "none"
#define NETDATA_WIN_CONTAINER_KUBERNETES          "container"
#define NETDATA_WIN_CONTAINER_KUBERNETES_DETECT   "kubernetes"
#define NETDATA_WIN_CONTAINER_WINDOWS             "windows-container"
#define NETDATA_WIN_CONTAINER_WINDOWS_DETECT      "windows-api"

#ifdef OS_WINDOWS
#include "windows.h"
#include "versionhelpers.h"

typedef struct netdata_windows_os_labels {
    char name[64];
    char version[64];
    char release[64];
    char edition[256];
    char build[64];
} NETDATA_WINDOWS_OS_LABELS;

void netdata_windows_get_system_info(struct rrdhost_system_info *system_info);

const char *netdata_windows_normalize_virt_string(const char *raw);
const char *netdata_windows_resolve_virt_detection(const char *wmi, const char *smbios, const char *registry);
void netdata_windows_format_os_version(char *out, size_t length, const char *product_name, DWORD build, bool is_server);
void netdata_windows_format_os_id_like(char *out, size_t length, DWORD build, const char *edition_id,
                                       bool is_server);
void netdata_windows_parse_os_labels(NETDATA_WINDOWS_OS_LABELS *labels, const char *product_name,
                                     const char *display_version, const char *edition_id, DWORD build,
                                     DWORD ubr, bool has_ubr, bool is_server);

// Kubernetes env-var container probe: returns NETDATA_WIN_CONTAINER_KUBERNETES when both
// values are set and non-empty, otherwise NULL (caller should fall back to WMI detection).
const char *netdata_windows_container_from_env(const char *k_host, const char *k_port);

// Maps a container classification to its detection-method string.
const char *netdata_windows_container_detection_method(const char *container);
#endif

#endif // _NETDATA_WIN_SYSTEM_INFO_H_
