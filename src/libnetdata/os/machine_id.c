// SPDX-License-Identifier: GPL-3.0-or-later

#include "machine_id.h"
#include "libnetdata/libnetdata.h"

static ND_UUID cached_machine_id = { 0 };
static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

#if defined(OS_LINUX)

static ND_UUID get_machine_id(void) {
    ND_UUID machine_id = { 0 };

    // Try systemd machine-id locations first
    const char *locations[] = {
        "/etc/machine-id",                  // systemd standard location
        "/var/lib/dbus/machine-id",         // fallback for older systems or different distros
        "/sys/class/dmi/id/product_uuid"    // hardware-based UUID from DMI
    };

    for (size_t i = 0; i < _countof(locations); i++) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, sizeof(filename), "%s%s",
                  netdata_configured_host_prefix ? netdata_configured_host_prefix : "", locations[i]);

        char buf[128];
        if (read_txt_file(filename, buf, sizeof(buf)) == 0) {
            if (uuid_parse(trim(buf), machine_id.uuid) == 0)
                return machine_id;
        }
    }

    // If no reliable machine ID could be found, return NO_MACHINE_ID
    return NO_MACHINE_ID;
}

#elif defined(OS_FREEBSD)

static ND_UUID get_machine_id(void) {
    ND_UUID machine_id = { 0 };
    char buf[128];

    // Try FreeBSD host ID first
    char filename[FILENAME_MAX + 1];
    snprintfz(filename, sizeof(filename), "%s/etc/hostid",
              netdata_configured_host_prefix ? netdata_configured_host_prefix : "");

    if (read_txt_file(filename, buf, sizeof(buf)) == 0) {
        if (uuid_parse(trim(buf), machine_id.uuid) == 0)
            return machine_id;
    }

    // Fallback: Read system kern.hostuuid sysctl
    size_t len = sizeof(buf);
    if (sysctlbyname("kern.hostuuid", buf, &len, NULL, 0) == 0) {
        if (uuid_parse(trim(buf), machine_id.uuid) == 0)
            return machine_id;
    }

    // If no reliable machine ID could be found, return NO_MACHINE_ID
    return NO_MACHINE_ID;
}

#elif defined(OS_MACOS)

#include <IOKit/IOKitLib.h>

static ND_UUID get_machine_id(void) {
    ND_UUID machine_id = { 0 };

    // First try to get the platform UUID
    io_registry_entry_t ioRegistryRoot = IORegistryEntryFromPath(kIOMasterPortDefault, "IOService:/");
    if (ioRegistryRoot) {
        CFStringRef uuidCf = (CFStringRef) IORegistryEntryCreateCFProperty(
            ioRegistryRoot,
            CFSTR(kIOPlatformUUIDKey),
            kCFAllocatorDefault,
            0);

        if (uuidCf) {
            char uuid_str[UUID_STR_LEN];
            if (CFStringGetCString(uuidCf, uuid_str, sizeof(uuid_str), kCFStringEncodingUTF8)) {
                if (uuid_parse(uuid_str, machine_id.uuid) == 0) {
                    CFRelease(uuidCf);
                    IOObjectRelease(ioRegistryRoot);
                    return machine_id;
                }
            }
            CFRelease(uuidCf);
        }
        IOObjectRelease(ioRegistryRoot);
    }

    // Fallback to IOPlatformExpertDevice's IOPlatformSerialNumber
    io_service_t platformExpert = IOServiceGetMatchingService(
        kIOMasterPortDefault,
        IOServiceMatching("IOPlatformExpertDevice"));

    if (platformExpert) {
        CFStringRef serialNumberCf = (CFStringRef) IORegistryEntryCreateCFProperty(
            platformExpert,
            CFSTR(kIOPlatformSerialNumberKey),
            kCFAllocatorDefault,
            0);

        if (serialNumberCf) {
            char serial_str[128];
            if (CFStringGetCString(serialNumberCf, serial_str, sizeof(serial_str), kCFStringEncodingUTF8)) {
                // Hardware serial numbers are considered reliable and stable
                if (strlen(serial_str) > 0) {
                    // Generate a UUID from the serial number
                    char uuid_input[150];
                    snprintfz(uuid_input, sizeof(uuid_input), "mac-serial:%s", serial_str);
                    machine_id = UUID_generate_from_hash(uuid_input, strlen(uuid_input));

                    CFRelease(serialNumberCf);
                    IOObjectRelease(platformExpert);
                    return machine_id;
                }
            }
            CFRelease(serialNumberCf);
        }
        IOObjectRelease(platformExpert);
    }

    // If no reliable machine ID could be found, return NO_MACHINE_ID
    return NO_MACHINE_ID;
}

#elif defined(OS_WINDOWS)
#include <windows.h>

static ND_UUID get_machine_id(void) {
    ND_UUID machine_id = { 0 };
    HKEY hKey;

    // Try to get MachineGuid from registry - this is the most reliable machine ID on Windows
    if (RegOpenKeyExW(HKEY_LOCAL_MACHINE, L"SOFTWARE\\Microsoft\\Cryptography", 0, KEY_READ, &hKey) == ERROR_SUCCESS) {
        WCHAR guidW[64];
        DWORD guidSize = sizeof(guidW);
        DWORD type = REG_SZ;

        if (RegQueryValueExW(hKey, L"MachineGuid", NULL, &type, (LPBYTE)guidW, &guidSize) == ERROR_SUCCESS) {
            char guid_str[UUID_STR_LEN];

            // Convert GUID to UTF-8
            if (WideCharToMultiByte(CP_UTF8, 0, guidW, -1, guid_str, sizeof(guid_str), NULL, NULL) > 0) {
                if (uuid_parse(guid_str, machine_id.uuid) == 0) {
                    RegCloseKey(hKey);
                    return machine_id;
                }
            }
        }
        RegCloseKey(hKey);
    }

    // If no reliable machine ID could be found, return NO_MACHINE_ID
    return NO_MACHINE_ID;
}

#endif // OS_WINDOWS

ND_UUID os_machine_id(void) {
    // Fast path - return cached value if available
    if(!UUIDiszero(cached_machine_id))
        return cached_machine_id;

    spinlock_lock(&spinlock);

    // Check again under lock in case another thread set it
    if(UUIDiszero(cached_machine_id)) {
        cached_machine_id = get_machine_id();

        // Log the result if debugging is enabled
        if(UUIDeq(cached_machine_id, NO_MACHINE_ID))
            nd_log(NDLS_DAEMON, NDLP_WARNING, "OS_MACHINE_ID: Could not detect a reliable machine ID");
        else {
            char buf[UUID_STR_LEN];
            uuid_unparse_lower(cached_machine_id.uuid, buf);
            nd_log(NDLS_DAEMON, NDLP_NOTICE, "OS_MACHINE_ID: machine ID found '%s'", buf);
        }
    }

    spinlock_unlock(&spinlock);
    return cached_machine_id;
}
