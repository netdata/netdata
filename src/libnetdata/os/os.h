// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_OS_H
#define NETDATA_OS_H

#if defined(OS_LINUX) || defined(OS_FREEBSD) || defined(OS_MACOS)
#include <sys/syscall.h>
#endif

#include "system_memory.h"
#include "random.h"
#include "timestamps.h"
#include "setproctitle.h"
#include "close_range.h"
#include "setresuid.h"
#include "setresgid.h"
#include "getgrouplist.h"
#include "adjtimex.h"
#include "gettid.h"
#include "get_pid_max.h"
#include "get_system_cpus.h"
#include "get_system_pagesize.h"
#include "sleep.h"
#include "uuid_generate.h"
#include "setenv.h"
#include "hostname.h"
#include "os-freebsd-wrappers.h"
#include "os-macos-wrappers.h"
#include "os-windows-wrappers.h"
#include "system-maps/cached-uid-username.h"
#include "system-maps/cached-gid-groupname.h"
#include "system-maps/cache-host-users-and-groups.h"
#include "system-maps/cached-sid-username.h"
#include "windows-perflib/perflib.h"
#include "disk_space.h"
#include "file_metadata.h"
#include "process_path.h"
#include "boottime.h"
#include "boot_id.h"
#include "run_dir.h"
#include "file_lock.h"
#include "mmap_limit.h"

// this includes windows.h to the whole of netdata
// so various conflicts arise
// #include "windows-wmi/windows-wmi.h"

// =====================================================================================================================
// common defs for Apple/FreeBSD/Linux

extern const char *os_type;

extern unsigned int system_hz;
void os_get_system_HZ(void);

#endif //NETDATA_OS_H
