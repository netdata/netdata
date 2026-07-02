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

#define NETDATA_WIN_CONTAINER_NONE                "none"
#define NETDATA_WIN_CONTAINER_KUBERNETES          "container"
#define NETDATA_WIN_CONTAINER_KUBERNETES_DETECT   "kubernetes"
#define NETDATA_WIN_CONTAINER_WINDOWS             "windows-container"
#define NETDATA_WIN_CONTAINER_WINDOWS_DETECT      "windows-api"

#ifdef OS_WINDOWS
#include "windows.h"
#include "versionhelpers.h"

void netdata_windows_get_system_info(struct rrdhost_system_info *system_info);

const char *netdata_windows_normalize_virt_string(const char *raw);
const char *netdata_windows_resolve_virt_detection(const char *wmi, const char *smbios, const char *registry);
#endif

#endif // _NETDATA_WIN_SYSTEM_INFO_H_
