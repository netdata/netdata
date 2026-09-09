// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PLUGIN_MACOS_H
#define NETDATA_PLUGIN_MACOS_H 1

#include "database/rrd.h"

#include <AvailabilityMacros.h>

// macOS 12 renamed IOMasterPort()/kIOMasterPortDefault to IOMainPort()/kIOMainPortDefault.
// Alias the modern names back onto the legacy ones so the plugin builds against older SDKs.
// Keyed on MAX_ALLOWED (the SDK), matching src/daemon/status-file-dmi.c and
// src/libnetdata/os/machine_id.c: an SDK old enough to need the alias also has no conflicting
// declaration of the modern names, so defining these before <IOKit/IOKitLib.h> is safe.
#if !defined(MAC_OS_VERSION_12_0) || (MAC_OS_X_VERSION_MAX_ALLOWED < MAC_OS_VERSION_12_0)
#define IOMainPort IOMasterPort
#define kIOMainPortDefault kIOMasterPortDefault
#endif

int do_macos_sysctl(int update_every, usec_t dt);
int do_macos_mach_smi(int update_every, usec_t dt);
int do_macos_iokit(int update_every, usec_t dt);
int do_macos_power_sources(int update_every, usec_t dt);
int do_macos_gpu(int update_every, usec_t dt);
int do_macos_sensors(int update_every, usec_t dt);
int do_macos_powermetrics(int update_every, usec_t dt);
int do_macos_nvme_smart(int update_every, usec_t dt);

bool macos_gpu_power_available(void);
bool macos_gpu_power_source_available(void);
bool macos_gpu_temperature_available(void);
bool macos_gpu_is_hid_temperature_sensor_name(const char *name);
bool macos_sensors_gpu_temperature_available(void);
bool macos_sensors_fan_available(void);
void macos_powermetrics_release_gpu_power_fallback(void);
void macos_powermetrics_release_gpu_temperature_fallback(void);
void macos_gpu_cleanup(void);
void macos_iokit_cleanup(void);
void macos_sensors_cleanup(void);
void macos_powermetrics_cleanup(void);
void macos_nvme_smart_cleanup(void);
void macos_power_sources_cleanup(void);

#endif /* NETDATA_PLUGIN_MACOS_H */
