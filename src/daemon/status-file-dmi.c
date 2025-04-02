// SPDX-License-Identifier: GPL-3.0-or-later

#include "status-file-dmi.h"

static void dmi_clean_field_placeholder(char *buf, size_t buf_size) {
    if(!buf || !buf_size) return;

    struct {
        const char *found;
        const char *replace;
    } placeholders[] = {
        {"$(DEFAULT_STRING)", ""},
        {"Chassis Manufacture", ""},
        {"Chassis Manufacturer", ""},
        {"Chassis Version", ""},
        {"Default string", ""},
        {"N/A", ""},
        {"NA", ""},
        {"NOT SPECIFIED", ""},
        {"No Enclosure", ""},
        {"None Provided", ""},
        {"None", ""},
        {"OEM Chassis Manufacturer", ""},
        {"OEM Default string000", ""},
        {"OEM", ""},
        {"OEM_MB", ""},
        {"SYSTEM_MANUFACTURER", ""},
        {"SmbiosType1_SystemManufacturer", ""},
        {"SmbiosType2_BoardManufacturer", ""},
        {"Standard", ""},
        {"System Product Name", ""},
        {"System UUID", ""},
        {"System Version", ""},
        {"System manufacturer", ""},
        {"TBD by OEM", ""},
        {"TBD", ""},
        {"To be filled by O.E.M.", ""},
        {"Type2 - Board Manufacturer", ""},
        {"Type2 - Board Vendor Name1", ""},
        {"Unknow", ""},
        {"Unknown", ""},
        {"XXXXX", ""},
        {"default", ""},
        {"empty", ""},
        {"unspecified", ""},
        {"x.x", ""},
        {"(null)", ""},
    };

    for (size_t i = 0; i < _countof(placeholders); i++) {
        if (strcasecmp(buf, placeholders[i].found) == 0) {
            strncpyz(buf, placeholders[i].replace, buf_size - 1);
            break;
        }
    }
}

static void dmi_normalize_vendor_field(char *buf, size_t buf_size) {
    if(!buf || !buf_size) return;

    struct {
        const char *found;
        const char *replace;
    } vendors[] = {
        // Major vendors with multiple variations
        {"AMD Corporation", "AMD"},
        {"Advanced Micro Devices, Inc.", "AMD"},

        {"AMI Corp.", "AMI"},
        {"AMI Corporation", "AMI"},
        {"American Megatrends", "AMI"},
        {"American Megatrends Inc.", "AMI"},
        {"American Megatrends International", "AMI"},
        {"American Megatrends International, LLC.", "AMI"},

        {"AOPEN", "AOpen"},
        {"AOPEN Inc.", "AOpen"},

        {"Apache Software Foundation", "Apache"},

        {"Apple Inc.", "Apple"},

        {"ASRock Industrial", "ASRock"},
        {"ASRockRack", "ASRock"},
        {"AsrockRack", "ASRock"},

        {"ASUS", "ASUSTeK"},
        {"ASUSTeK COMPUTER INC.", "ASUSTeK"},
        {"ASUSTeK COMPUTER INC. (Licensed from AMI)", "ASUSTeK"},
        {"ASUSTeK Computer INC.", "ASUSTeK"},
        {"ASUSTeK Computer Inc.", "ASUSTeK"},
        {"ASUSTek Computer INC.", "ASUSTeK"},

        {"Apache Software Foundation", "Apache"},

        {"BESSTAR (HK) LIMITED", "Besstar"},
        {"BESSTAR TECH", "Besstar"},
        {"BESSTAR TECH LIMITED", "Besstar"},
        {"BESSTAR Tech", "Besstar"},

        {"CHUWI", "Chuwi"},
        {"CHUWI Innovation And Technology(ShenZhen)co.,Ltd", "Chuwi"},

        {"Cisco Systems Inc", "Cisco"},
        {"Cisco Systems, Inc.", "Cisco"},

        {"DELL", "Dell"},
        {"Dell Computer Corporation", "Dell"},
        {"Dell Inc.", "Dell"},

        {"FUJITSU", "Fujitsu"},
        {"FUJITSU CLIENT COMPUTING LIMITED", "Fujitsu"},
        {"FUJITSU SIEMENS", "Fujitsu"},
        {"FUJITSU SIEMENS // Phoenix Technologies Ltd.", "Fujitsu"},
        {"FUJITSU // American Megatrends Inc.", "Fujitsu"},
        {"FUJITSU // American Megatrends International, LLC.", "Fujitsu"},
        {"FUJITSU // Insyde Software Corp.", "Fujitsu"},
        {"FUJITSU // Phoenix Technologies Ltd.", "Fujitsu"},

        {"GIGABYTE", "Gigabyte"},
        {"Giga Computing", "Gigabyte"},
        {"Gigabyte Technology Co., Ltd.", "Gigabyte"},
        {"Gigabyte Tecohnology Co., Ltd.", "Gigabyte"},

        {"GOOGLE", "Google"},

        {"HC Technology.,Ltd.", "HC Tech"},

        {"HP-Pavilion", "HP"},
        {"HPE", "HP"},
        {"Hewlett Packard Enterprise", "HP"},
        {"Hewlett-Packard", "HP"},

        {"HUAWEI", "Huawei"},
        {"Huawei Technologies Co., Ltd.", "Huawei"},

        {"IBM Corp.", "IBM"},

        {"INSYDE", "Insyde"},
        {"INSYDE Corp.", "Insyde"},
        {"Insyde Corp.", "Insyde"},

        {"INTEL", "Intel"},
        {"INTEL Corporation", "Intel"},
        {"Intel Corp.", "Intel"},
        {"Intel Corporation", "Intel"},
        {"Intel corporation", "Intel"},
        {"Intel(R) Client Systems", "Intel"},
        {"Intel(R) Corporation", "Intel"},

        {"LENOVO", "Lenovo"},
        {"LNVO", "Lenovo"},

        {"MICRO-STAR INTERNATIONAL CO., LTD", "MSI"},
        {"MICRO-STAR INTERNATIONAL CO.,LTD", "MSI"},
        {"MSI", "MSI"},
        {"Micro-Star International Co., Ltd", "MSI"},
        {"Micro-Star International Co., Ltd.", "MSI"},

        {"Microsoft Corporation", "Microsoft"},

        {"nVIDIA", "NVIDIA"},

        {"ORACLE CORPORATI", "Oracle"},
        {"Oracle Corporation", "Oracle"},
        {"innotek GmbH", "Oracle"},

        {"Phoenix Technologies LTD", "Phoenix"},
        {"Phoenix Technologies Ltd", "Phoenix"},
        {"Phoenix Technologies Ltd.", "Phoenix"},
        {"Phoenix Technologies, LTD", "Phoenix"},

        {"QNAP Systems, Inc.", "QNAP"},

        {"QUANTA", "Quanta"},
        {"Quanta Cloud Technology Inc.", "Quanta"},
        {"Quanta Computer Inc", "Quanta"},
        {"Quanta Computer Inc.", "Quanta"},

        {"RED HAT", "Red Hat"},

        {"SAMSUNG ELECTRONICS CO., LTD.", "Samsung"},

        {"SuperMicro", "Supermicro"},
        {"Supermicro Corporation", "Supermicro"},

        {"SYNOLOGY", "Synology"},
        {"Synology Inc.", "Synology"},

        {"TYAN", "Tyan"},
        {"TYAN Computer Corporation", "Tyan"},
        {"Tyan Computer Corporation", "Tyan"},
        {"$(TYAN_SYSTEM_MANUFACTURER)", "Tyan"},

        {"VMware", "VMware"},
        {"VMware, Inc.", "VMware"},

        {"XIAOMI", "Xiaomi"},

        {"ZOTAC", "Zotac"},
        {"Motherboard by ZOTAC", "Zotac"}
    };

    for (size_t i = 0; i < _countof(vendors); i++) {
        if (strcasecmp(buf, vendors[i].found) == 0) {
            strncpyz(buf, vendors[i].replace, buf_size - 1);
            break;
        }
    }
}

static bool dmi_is_virtual_machine(const DAEMON_STATUS_FILE *ds) {
    if(!ds) return false;

    const char *vm_indicators[] = {
        "Virt", "KVM", "vServer", "Cloud", "Hyper", "Droplet", "Compute",
        "HVM domU", "Parallels", "(i440FX", "(q35", "OpenStack", "QEMU",
        "VMWare", "DigitalOcean", "Oracle", "Linode", "Amazon EC2"
    };

    const char *strs_to_check[] = {
        ds->hw.product.name,
        ds->hw.product.family,
        ds->hw.sys.vendor,
        ds->hw.board.name,
    };

    for (size_t i = 0; i < _countof(strs_to_check); i++) {
        if (!strs_to_check[i] || !strs_to_check[i][0])
            continue;

        for (size_t j = 0; j < _countof(vm_indicators); j++) {
            if (strcasestr(strs_to_check[i], vm_indicators[j]) != NULL)
                return true;
        }
    }

    return false;
}

static const char *dmi_chassis_type_to_string(int chassis_type) {
    // Original info from SMBIOS
    // https://www.dmtf.org/sites/default/files/standards/documents/DSP0134_3.2.0.pdf
    // selected values aligned with inxi: https://github.com/smxi/inxi/blob/master/inxi
    switch(chassis_type) {
        case 1: return "other";
        case 2: return "unknown";
        case 3: return "desktop";
        case 4: return "desktop"; /* "low-profile-desktop" */
        case 5: return "pizza-box"; // was a 1 U desktop enclosure, but some old laptops also id this way
        case 6: return "desktop"; /* "mini-tower-desktop" */
        case 7: return "desktop"; /* "tower-desktop" */
        case 8: return "portable";
        case 9: return "laptop";
        case 10: return "laptop"; /* "notebook" */
        case 11: return "portable"; /* "hand-held" */
        case 12: return "docking-station";
        case 13: return "desktop"; /* "all-in-one" */
        case 14: return "notebook"; /* "sub-notebook" */
        case 15: return "desktop"; /* "space-saving-desktop" */
        case 16: return "laptop"; /* "lunch-box" */
        case 17: return "server"; /* "main-server-chassis" */
        case 18: return "expansion-chassis";
        case 19: return "sub-chassis";
        case 20: return "bus-expansion";
        case 21: return "peripheral";
        case 22: return "raid";
        case 23: return "server"; /* "rack-mount-server" */
        case 24: return "desktop"; /* "sealed-desktop" */
        case 25: return "multimount-chassis";
        case 26: return "compact-pci";
        case 27: return "blade"; /* "advanced-tca" */
        case 28: return "blade";
        case 29: return "blade-enclosure";
        case 30: return "tablet";
        case 31: return "convertible";
        case 32: return "detachable";
        case 33: return "iot-gateway";
        case 34: return "embedded-pc";
        case 35: return "mini-pc";
        case 36: return "stick-pc";
        default: return NULL; // let it be numeric
    }
}

static void dmi_map_chassis_type(DAEMON_STATUS_FILE *ds, int chassis_type) {
    if(!ds) return;

    const char *str = NULL;

    if(dmi_is_virtual_machine(ds))
        str = "vm";

    if(!str)
        str = dmi_chassis_type_to_string(chassis_type);

    if(str)
        strncpyz(ds->hw.chassis.type, str, sizeof(ds->hw.chassis.type) - 1);
}

static void dmi_clean_field(char *buf, size_t buf_size) {
    if(!buf || !buf_size) return;

    // replace non-ascii characters and control characters with spaces
    for (size_t i = 0; i < sizeof(buf) && buf[i]; i++) {
        if (!isascii((uint8_t)buf[i]) || iscntrl((uint8_t)buf[i]))
            buf[i] = ' ';
    }

    // detect if all characters are symbols
    bool contains_alnum = false;
    for (size_t i = 0; i < sizeof(buf) && buf[i]; i++) {
        if (isalnum((uint8_t)buf[i])) {
            contains_alnum = true;
            break;
        }
    }
    if (!contains_alnum || !buf[0])
        return;

    // remove leading, trailing and duplicate spaces
    trim_all(buf);
    if (!buf[0])
        return;

    dmi_clean_field_placeholder(buf, buf_size);
}

// --------------------------------------------------------------------------------------------------------------------

#if defined(OS_LINUX)
static void linux_get_dmi_field(const char *field, const char *alt, char *dst, size_t dst_size) {
    char filename[FILENAME_MAX];
    dst[0] = '\0';

    if (netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        snprintfz(filename, sizeof(filename), "%s/sys/class/dmi/id/%s", netdata_configured_host_prefix, field);
        if (access(filename, R_OK) != 0) {
            snprintfz(
                filename, sizeof(filename), "%s/sys/devices/virtual/dmi/id/%s", netdata_configured_host_prefix, field);
            if (access(filename, R_OK) != 0)
                filename[0] = '\0';
        }
    } else
        filename[0] = '\0';

    if (!filename[0]) {
        snprintfz(filename, sizeof(filename), "/sys/class/dmi/id/%s", field);
        if (access(filename, R_OK) != 0) {
            snprintfz(filename, sizeof(filename), "/sys/devices/virtual/dmi/id/%s", field);
            if (access(filename, R_OK) != 0) {
                if (alt && *alt) {
                    strncpyz(filename, alt, sizeof(filename) - 1);
                    if (access(filename, R_OK) != 0)
                        filename[0] = '\0';
                } else
                    filename[0] = '\0';
            }
        }
    }

    if (!filename[0])
        return;

    char buf[256];
    if (read_txt_file(filename, buf, sizeof(buf)) != 0)
        return;

    if (!buf[0])
        return;

    dmi_clean_field(buf, sizeof(buf));

    if (!buf[0])
        return;

    // copy it to its final location
    strncpyz(dst, buf, dst_size - 1);
}

void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    linux_get_dmi_field("sys_vendor", NULL, ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));

    linux_get_dmi_field("product_name", "/proc/device-tree/model", ds->hw.product.name, sizeof(ds->hw.product.name));
    linux_get_dmi_field("product_version", NULL, ds->hw.product.version, sizeof(ds->hw.product.version));
    linux_get_dmi_field("product_sku", NULL, ds->hw.product.sku, sizeof(ds->hw.product.sku));
    linux_get_dmi_field("product_family", NULL, ds->hw.product.family, sizeof(ds->hw.product.family));

    linux_get_dmi_field("chassis_vendor", NULL, ds->hw.chassis.vendor, sizeof(ds->hw.chassis.vendor));
    linux_get_dmi_field("chassis_version", NULL, ds->hw.chassis.version, sizeof(ds->hw.chassis.version));

    linux_get_dmi_field("board_vendor", NULL, ds->hw.board.vendor, sizeof(ds->hw.board.vendor));
    linux_get_dmi_field("board_name", NULL, ds->hw.board.name, sizeof(ds->hw.board.name));
    linux_get_dmi_field("board_version", NULL, ds->hw.board.version, sizeof(ds->hw.board.version));

    linux_get_dmi_field("bios_vendor", NULL, ds->hw.bios.vendor, sizeof(ds->hw.bios.vendor));
    linux_get_dmi_field("bios_version", NULL, ds->hw.bios.version, sizeof(ds->hw.bios.version));
    linux_get_dmi_field("bios_date", NULL, ds->hw.bios.date, sizeof(ds->hw.bios.date));
    linux_get_dmi_field("bios_release", NULL, ds->hw.bios.release, sizeof(ds->hw.bios.release));

    linux_get_dmi_field("chassis_type", NULL, ds->hw.chassis.type, sizeof(ds->hw.chassis.type));
}
#elif defined(OS_MACOS)
#include <IOKit/IOKitLib.h>
#include <CoreFoundation/CoreFoundation.h>

static void macos_get_string_property(io_registry_entry_t entry, CFStringRef key, char *dst, size_t dst_size) {
    CFTypeRef property = IORegistryEntryCreateCFProperty(entry, key, kCFAllocatorDefault, 0);
    if (property) {
        if (CFGetTypeID(property) == CFStringGetTypeID()) {
            CFStringRef stringRef = (CFStringRef)property;
            if (!CFStringGetCString(stringRef, dst, dst_size, kCFStringEncodingUTF8)) {
                dst[0] = '\0';
            }
        }
        CFRelease(property);
    }
}

void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    // Get system information from IOKit
    io_registry_entry_t platformExpert = IORegistryEntryFromPath(
        kIOMasterPortDefault, "IOService:/IOResources/IOPlatformExpertDevice"
    );

    if (!platformExpert) {
        return;
    }

    // System vendor (usually Apple)
    macos_get_string_property(platformExpert, CFSTR("manufacturer"), ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));

    // Product information
    macos_get_string_property(platformExpert, CFSTR("model"), ds->hw.product.name, sizeof(ds->hw.product.name));
    macos_get_string_property(platformExpert, CFSTR("version"), ds->hw.product.version, sizeof(ds->hw.product.version));

    // If product name is empty, try to get machine model
    if (!ds->hw.product.name[0]) {
        char model[256] = "";
        size_t len = sizeof(model);
        if (sysctlbyname("hw.model", model, &len, NULL, 0) == 0) {
            dmi_clean_field(model, sizeof(model));
            strncpyz(ds->hw.product.name, model, sizeof(ds->hw.product.name) - 1);
        }
    }

    // Determine chassis type based on product name
    // Use IOPlatformExpertDevice to determine device type
    CFTypeRef deviceTypeProperty = IORegistryEntryCreateCFProperty(
        platformExpert, CFSTR("device_type"), kCFAllocatorDefault, 0
    );

    if (deviceTypeProperty && CFGetTypeID(deviceTypeProperty) == CFStringGetTypeID()) {
        char device_type[256];
        if (CFStringGetCString((CFStringRef)deviceTypeProperty, device_type, sizeof(device_type), kCFStringEncodingUTF8)) {
            // Set chassis type based on device type
            if (strstr(device_type, "MacBook") != NULL) {
                strncpyz(ds->hw.chassis.type, "laptop", sizeof(ds->hw.chassis.type) - 1);
            } else if (strstr(device_type, "iMac") != NULL) {
                strncpyz(ds->hw.chassis.type, "desktop", sizeof(ds->hw.chassis.type) - 1);
            } else if (strstr(device_type, "Mac") != NULL && strstr(device_type, "Pro") != NULL) {
                strncpyz(ds->hw.chassis.type, "desktop", sizeof(ds->hw.chassis.type) - 1);
            } else if (strstr(device_type, "Xserve") != NULL) {
                strncpyz(ds->hw.chassis.type, "server", sizeof(ds->hw.chassis.type) - 1);
            } else {
                strncpyz(ds->hw.chassis.type, "unknown", sizeof(ds->hw.chassis.type) - 1);
            }
        }
        CFRelease(deviceTypeProperty);
    }

    // BIOS information (on Mac this is firmware/SMC info)
    io_registry_entry_t romEntry = IORegistryEntryFromPath(
        kIOMasterPortDefault, "IODeviceTree:/rom"
    );

    if (romEntry) {
        macos_get_string_property(romEntry, CFSTR("vendor"), ds->hw.bios.vendor, sizeof(ds->hw.bios.vendor));
        macos_get_string_property(romEntry, CFSTR("version"), ds->hw.bios.version, sizeof(ds->hw.bios.version));
        macos_get_string_property(romEntry, CFSTR("release-date"), ds->hw.bios.date, sizeof(ds->hw.bios.date));
        IOObjectRelease(romEntry);
    }

    // Default Apple as vendor if not set
    if (!ds->hw.sys.vendor[0]) {
        strncpyz(ds->hw.sys.vendor, "Apple", sizeof(ds->hw.sys.vendor) - 1);
    }

    // Board info is generally not available on macOS
    // Use product name for board name if empty
    if (!ds->hw.board.name[0] && ds->hw.product.name[0]) {
        strncpyz(ds->hw.board.name, ds->hw.product.name, sizeof(ds->hw.board.name) - 1);
    }

    if (!ds->hw.board.vendor[0] && ds->hw.sys.vendor[0]) {
        strncpyz(ds->hw.board.vendor, ds->hw.sys.vendor, sizeof(ds->hw.board.vendor) - 1);
    }

    // Clean up fields
    dmi_clean_field(ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));
    dmi_clean_field(ds->hw.product.name, sizeof(ds->hw.product.name));
    dmi_clean_field(ds->hw.product.version, sizeof(ds->hw.product.version));
    dmi_clean_field(ds->hw.chassis.type, sizeof(ds->hw.chassis.type));
    dmi_clean_field(ds->hw.board.name, sizeof(ds->hw.board.name));
    dmi_clean_field(ds->hw.bios.vendor, sizeof(ds->hw.bios.vendor));
    dmi_clean_field(ds->hw.bios.version, sizeof(ds->hw.bios.version));
    dmi_clean_field(ds->hw.bios.date, sizeof(ds->hw.bios.date));

    IOObjectRelease(platformExpert);
}
#elif defined(OS_FREEBSD)
#include <sys/types.h>
#include <sys/sysctl.h>
#include <kenv.h>

static void freebsd_get_sysctl_str(const char *name, char *dst, size_t dst_size) {
    size_t len = dst_size;
    if (sysctlbyname(name, dst, &len, NULL, 0) == 0) {
        dst[len] = '\0';  // Ensure null termination
        dmi_clean_field(dst, dst_size);
    } else {
        dst[0] = '\0';
    }
}

static void freebsd_get_kenv_str(const char *name, char *dst, size_t dst_size) {
    if (kenv(KENV_GET, name, dst, dst_size) == -1) {
        dst[0] = '\0';
    } else {
        dmi_clean_field(dst, dst_size);
    }
}

void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    // System information from SMBIOS
    freebsd_get_sysctl_str("hw.vendor", ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));
    freebsd_get_sysctl_str("hw.product", ds->hw.product.name, sizeof(ds->hw.product.name));
    freebsd_get_sysctl_str("hw.version", ds->hw.product.version, sizeof(ds->hw.product.version));

    // Try using kenv for additional information
    freebsd_get_kenv_str("smbios.system.maker", ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));
    freebsd_get_kenv_str("smbios.system.product", ds->hw.product.name, sizeof(ds->hw.product.name));
    freebsd_get_kenv_str("smbios.system.version", ds->hw.product.version, sizeof(ds->hw.product.version));
    freebsd_get_kenv_str("smbios.system.sku", ds->hw.product.sku, sizeof(ds->hw.product.sku));
    freebsd_get_kenv_str("smbios.system.family", ds->hw.product.family, sizeof(ds->hw.product.family));

    // Board information
    freebsd_get_kenv_str("smbios.planar.maker", ds->hw.board.vendor, sizeof(ds->hw.board.vendor));
    freebsd_get_kenv_str("smbios.planar.product", ds->hw.board.name, sizeof(ds->hw.board.name));
    freebsd_get_kenv_str("smbios.planar.version", ds->hw.board.version, sizeof(ds->hw.board.version));

    // BIOS information
    freebsd_get_kenv_str("smbios.bios.vendor", ds->hw.bios.vendor, sizeof(ds->hw.bios.vendor));
    freebsd_get_kenv_str("smbios.bios.version", ds->hw.bios.version, sizeof(ds->hw.bios.version));
    freebsd_get_kenv_str("smbios.bios.reldate", ds->hw.bios.date, sizeof(ds->hw.bios.date));
    freebsd_get_kenv_str("smbios.bios.release", ds->hw.bios.release, sizeof(ds->hw.bios.release));

    // Chassis information
    freebsd_get_kenv_str("smbios.chassis.maker", ds->hw.chassis.vendor, sizeof(ds->hw.chassis.vendor));
    freebsd_get_kenv_str("smbios.chassis.version", ds->hw.chassis.version, sizeof(ds->hw.chassis.version));

    // Chassis type
    char chassis_type[16] = "";
    freebsd_get_kenv_str("smbios.chassis.type", chassis_type, sizeof(chassis_type));
    if (chassis_type[0]) {
        int type = atoi(chassis_type);
        if (type > 0) {
            snprintf(ds->hw.chassis.type, sizeof(ds->hw.chassis.type), "%d", type);
        }
    }

    // If we couldn't get system information from SMBIOS, try to use model
    if (!ds->hw.product.name[0]) {
        freebsd_get_sysctl_str("hw.model", ds->hw.product.name, sizeof(ds->hw.product.name));
    }
}
#elif defined(OS_WINDOWS)

#include <windows.h>
#include <wbemidl.h>
#include <oleauto.h>
#include <comdef.h>

#pragma comment(lib, "wbemuuid.lib")
#pragma comment(lib, "ole32.lib")
#pragma comment(lib, "oleaut32.lib")

// Helper function to initialize COM and WMI
static HRESULT windows_init_wmi(IWbemServices **services) {
    HRESULT hr;
    IWbemLocator *locator = NULL;

    // Initialize COM
    hr = CoInitializeEx(0, COINIT_MULTITHREADED);
    if (FAILED(hr) && hr != RPC_E_CHANGED_MODE) {
        return hr;
    }

    // Set security levels
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
        CoUninitialize();
        return hr;
    }

    // Create WMI locator
    hr = CoCreateInstance(
        &CLSID_WbemLocator,
        0,
        CLSCTX_INPROC_SERVER,
        &IID_IWbemLocator,
        (LPVOID *)&locator
    );

    if (FAILED(hr)) {
        CoUninitialize();
        return hr;
    }

    // Connect to WMI namespace
    hr = locator->lpVtbl->ConnectServer(
        locator,
        L"ROOT\\CIMV2",
        NULL,
        NULL,
        0,
        0,
        0,
        NULL,
        services
    );

    locator->lpVtbl->Release(locator);

    if (FAILED(hr)) {
        CoUninitialize();
        return hr;
    }

    // Set proxy security
    hr = CoSetProxyBlanket(
        (IUnknown *)*services,
        RPC_C_AUTHN_WINNT,
        RPC_C_AUTHZ_NONE,
        NULL,
        RPC_C_AUTHN_LEVEL_CALL,
        RPC_C_IMP_LEVEL_IMPERSONATE,
        NULL,
        EOAC_NONE
    );

    if (FAILED(hr)) {
        (*services)->lpVtbl->Release(*services);
        CoUninitialize();
        return hr;
    }

    return S_OK;
}

// Helper function to cleanup WMI resources
static void windows_cleanup_wmi(IWbemServices *services) {
    if (services) {
        services->lpVtbl->Release(services);
    }
    CoUninitialize();
}

// Convert BSTR to UTF-8 or ASCII
static void bstr_to_utf8(BSTR bstr, char *dst, size_t dst_size) {
    if (!bstr || !dst || dst_size == 0) {
        if (dst && dst_size > 0)
            dst[0] = '\0';
        return;
    }

    // Get required size for UTF-8 conversion
    int size_needed = WideCharToMultiByte(CP_UTF8, 0, bstr, -1, NULL, 0, NULL, NULL);

    if (size_needed <= 0) {
        dst[0] = '\0';
        return;
    }

    // Perform conversion to UTF-8
    if (WideCharToMultiByte(CP_UTF8, 0, bstr, -1, dst, dst_size, NULL, NULL) <= 0) {
        dst[0] = '\0';
    }
}

// Helper function to execute a WMI query and retrieve a string property
static void windows_wmi_query_string(IWbemServices *services, const wchar_t *query,
                                     const wchar_t *property, char *dst, size_t dst_size) {
    HRESULT hr;
    IEnumWbemClassObject *enumerator = NULL;
    IWbemClassObject *object = NULL;
    ULONG returned = 0;
    VARIANT value;

    // Set empty result by default
    dst[0] = '\0';

    // Execute the WMI query
    hr = services->lpVtbl->ExecQuery(
        services,
        L"WQL",
        (BSTR)query,
        WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
        NULL,
        &enumerator
    );

    if (FAILED(hr)) {
        return;
    }

    // Get the first object
    hr = enumerator->lpVtbl->Next(enumerator, WBEM_INFINITE, 1, &object, &returned);

    // If we got an object, get the property
    if (SUCCEEDED(hr) && returned > 0) {
        VariantInit(&value);
        hr = object->lpVtbl->Get(object, property, 0, &value, NULL, NULL);

        if (SUCCEEDED(hr) && value.vt == VT_BSTR) {
            // Convert BSTR to UTF-8
            bstr_to_utf8(value.bstrVal, dst, dst_size);
            dmi_clean_field(dst, dst_size);
        }

        VariantClear(&value);
        object->lpVtbl->Release(object);
    }

    enumerator->lpVtbl->Release(enumerator);
}

// Helper function to get a numeric chassis type from WMI
static int windows_wmi_get_chassis_type(IWbemServices *services) {
    HRESULT hr;
    IEnumWbemClassObject *enumerator = NULL;
    IWbemClassObject *object = NULL;
    ULONG returned = 0;
    VARIANT value;
    int chassis_type = 0;

    // Execute the WMI query for chassis type
    hr = services->lpVtbl->ExecQuery(
        services,
        L"WQL",
        L"SELECT ChassisTypes FROM Win32_SystemEnclosure",
        WBEM_FLAG_FORWARD_ONLY | WBEM_FLAG_RETURN_IMMEDIATELY,
        NULL,
        &enumerator
    );

    if (FAILED(hr)) {
        return 0;
    }

    // Get the first object
    hr = enumerator->lpVtbl->Next(enumerator, WBEM_INFINITE, 1, &object, &returned);

    // If we got an object, get the ChassisTypes property (which is an array)
    if (SUCCEEDED(hr) && returned > 0) {
        VariantInit(&value);
        hr = object->lpVtbl->Get(object, L"ChassisTypes", 0, &value, NULL, NULL);

        if (SUCCEEDED(hr) && value.vt == (VT_ARRAY | VT_I4)) {
            // Get the first value from the array
            SAFEARRAY *array = value.parray;
            long lowerBound, upperBound;
            SafeArrayGetLBound(array, 1, &lowerBound);
            SafeArrayGetUBound(array, 1, &upperBound);

            if (lowerBound <= upperBound) {
                LONG* pData;
                hr = SafeArrayAccessData(array, (void**)&pData);
                if (SUCCEEDED(hr)) {
                    chassis_type = pData[0];
                    SafeArrayUnaccessData(array);
                }
            }
        }

        VariantClear(&value);
        object->lpVtbl->Release(object);
    }

    enumerator->lpVtbl->Release(enumerator);
    return chassis_type;
}

// Function to parse a BIOS date in format YYYYMMDD to MM/DD/YYYY
static void windows_format_bios_date(char *date, size_t date_size) {
    if (strlen(date) >= 8) {
        char year[5] = {0};
        char month[3] = {0};
        char day[3] = {0};

        // Extract components
        strncpy(year, date, 4);
        strncpy(month, date + 4, 2);
        strncpy(day, date + 6, 2);

        // Create formatted date
        char formatted[16];
        snprintf(formatted, sizeof(formatted), "%s/%s/%s", month, day, year);

        strncpyz(date, formatted, date_size - 1);
    }
}

void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    IWbemServices *services = NULL;
    HRESULT hr = windows_init_wmi(&services);

    if (FAILED(hr)) {
        // WMI initialization failed, set defaults
        strncpyz(ds->hw.sys.vendor, "Unknown", sizeof(ds->hw.sys.vendor) - 1);
        strncpyz(ds->hw.product.name, "Unknown", sizeof(ds->hw.product.name) - 1);
        return;
    }

    // System and manufacturer info
    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, Model FROM Win32_ComputerSystem",
                             L"Manufacturer", ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));

    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, Model FROM Win32_ComputerSystem",
                             L"Model", ds->hw.product.name, sizeof(ds->hw.product.name));

    // Motherboard information
    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, Product, Version FROM Win32_BaseBoard",
                             L"Manufacturer", ds->hw.board.vendor, sizeof(ds->hw.board.vendor));

    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, Product, Version FROM Win32_BaseBoard",
                             L"Product", ds->hw.board.name, sizeof(ds->hw.board.name));

    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, Product, Version FROM Win32_BaseBoard",
                             L"Version", ds->hw.board.version, sizeof(ds->hw.board.version));

    // BIOS information
    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, SMBIOSBIOSVersion, ReleaseDate FROM Win32_BIOS",
                             L"Manufacturer", ds->hw.bios.vendor, sizeof(ds->hw.bios.vendor));

    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, SMBIOSBIOSVersion, ReleaseDate FROM Win32_BIOS",
                             L"SMBIOSBIOSVersion", ds->hw.bios.version, sizeof(ds->hw.bios.version));

    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, SMBIOSBIOSVersion, ReleaseDate FROM Win32_BIOS",
                             L"ReleaseDate", ds->hw.bios.date, sizeof(ds->hw.bios.date));

    // Format BIOS date if needed
    windows_format_bios_date(ds->hw.bios.date, sizeof(ds->hw.bios.date));

    // Chassis information
    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, Version FROM Win32_SystemEnclosure",
                             L"Manufacturer", ds->hw.chassis.vendor, sizeof(ds->hw.chassis.vendor));

    windows_wmi_query_string(services,
                             L"SELECT Manufacturer, Version FROM Win32_SystemEnclosure",
                             L"Version", ds->hw.chassis.version, sizeof(ds->hw.chassis.version));

    // Get chassis type
    int chassis_type = windows_wmi_get_chassis_type(services);
    if (chassis_type > 0) {
        snprintf(ds->hw.chassis.type, sizeof(ds->hw.chassis.type), "%d", chassis_type);
    }

    // Additional product information if available
    windows_wmi_query_string(services,
                             L"SELECT SKU, Family FROM Win32_ComputerSystem",
                             L"SKU", ds->hw.product.sku, sizeof(ds->hw.product.sku));

    windows_wmi_query_string(services,
                             L"SELECT SKU, Family FROM Win32_ComputerSystem",
                             L"Family", ds->hw.product.family, sizeof(ds->hw.product.family));

    // Cleanup WMI resources
    windows_cleanup_wmi(services);
}

#else
void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    ;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// public API

void finalize_vendor_product_vm(DAEMON_STATUS_FILE *ds) {
    if(ds->cloud_provider_type[0] && strcmp(ds->cloud_provider_type, "unknown") != 0)
        strncpyz(ds->hw.sys.vendor, ds->cloud_provider_type, sizeof(ds->hw.sys.vendor) - 1);

    if(ds->cloud_instance_type[0] && strcmp(ds->cloud_instance_type, "unknown") != 0)
        strncpyz(ds->hw.product.name, ds->cloud_instance_type, sizeof(ds->hw.product.name) - 1);

    if(ds->virtualization[0] && strcmp(ds->virtualization, "none") != 0 && strcmp(ds->virtualization, "unknown") != 0)
        strncpyz(ds->hw.chassis.type, "vm", sizeof(ds->hw.product.version) - 1);
}

void fill_dmi_info(DAEMON_STATUS_FILE *ds) {
    os_dmi_info(ds);

    dmi_normalize_vendor_field(ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));
    dmi_normalize_vendor_field(ds->hw.board.vendor, sizeof(ds->hw.board.vendor));
    dmi_normalize_vendor_field(ds->hw.chassis.vendor, sizeof(ds->hw.chassis.vendor));
    dmi_normalize_vendor_field(ds->hw.bios.vendor, sizeof(ds->hw.bios.vendor));

    dmi_map_chassis_type(ds, atoi(ds->hw.chassis.type));

    // make sure we have a system vendor
    if(!ds->hw.sys.vendor[0])
        strncpyz(ds->hw.sys.vendor, ds->hw.board.vendor, sizeof(ds->hw.sys.vendor) - 1);
    if(!ds->hw.sys.vendor[0])
        strncpyz(ds->hw.sys.vendor, ds->hw.chassis.vendor, sizeof(ds->hw.sys.vendor) - 1);
    if(!ds->hw.sys.vendor[0])
        strncpyz(ds->hw.sys.vendor, ds->hw.bios.vendor, sizeof(ds->hw.sys.vendor) - 1);
    if(!ds->hw.sys.vendor[0])
        strncpyz(ds->hw.sys.vendor, "Unknown", sizeof(ds->hw.sys.vendor) - 1);

    // make sure we have a product name
    if(!ds->hw.product.name[0])
        strncpyz(ds->hw.product.name, ds->hw.board.name, sizeof(ds->hw.product.name) - 1);
    if(!ds->hw.product.name[0])
        strncpyz(ds->hw.product.name, "Unknown", sizeof(ds->hw.product.name) - 1);

    // make sure the cloud provider and cloud instance loaded from system-info.sh are preferred
    finalize_vendor_product_vm(ds);
}
