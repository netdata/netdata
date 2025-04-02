// SPDX-License-Identifier: GPL-3.0-or-later

#include "status-file-dmi.h"

#define safecpy(dst, src) do {                                                                  \
    _Static_assert(sizeof(dst) != sizeof(char *),                                               \
                   "safecpy: dst must not be a pointer, but a buffer (e.g., char dst[SIZE])");  \
    strcatz(dst, 0, src, sizeof(dst));                                                          \
} while (0)

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
            strcatz(buf, 0, placeholders[i].replace, buf_size);
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
            strcatz(buf, 0, vendors[i].replace, buf_size);
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
        safecpy(ds->hw.chassis.type, str);
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
                    safecpy(filename, alt);
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
    strcatz(dst, 0, buf, dst_size);
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
#include <sys/sysctl.h>

// Helper function to safely convert CF types to C strings
static void cf_string_to_cstr(CFTypeRef cf_val, char *buffer, size_t buffer_size) {
    // Initialize buffer to empty string
    if (!buffer || buffer_size == 0) {
        return;
    }

    buffer[0] = '\0';

    // Safety check for null CF reference
    if (!cf_val) {
        return;
    }

    // Pre-set the last byte to ensure termination even if conversion fails
    buffer[buffer_size - 1] = '\0';

    if (CFGetTypeID(cf_val) == CFStringGetTypeID()) {
        CFStringRef str_ref = (CFStringRef)cf_val;
        // Use CFStringGetCString with len-1 to ensure space for null terminator
        if (!CFStringGetCString(str_ref, buffer, buffer_size - 1, kCFStringEncodingUTF8)) {
            buffer[0] = '\0'; // Reset on failure
        }
    } else if (CFGetTypeID(cf_val) == CFDataGetTypeID()) {
        CFDataRef data_ref = (CFDataRef)cf_val;
        CFIndex length = CFDataGetLength(data_ref);
        if (length > 0 && length < buffer_size - 1) {
            const UInt8 *bytes = CFDataGetBytePtr(data_ref);
            memcpy(buffer, bytes, length);
            buffer[length] = '\0';
        }
    }

    dmi_clean_field(buffer, buffer_size);
}

// Get a string property from an IOKit registry entry safely
static void get_iokit_string_property(io_registry_entry_t entry, CFStringRef key,
                                      char *buffer, size_t buffer_size) {
    // Initialize buffer to empty
    if (!buffer || buffer_size == 0 || !entry || !key) {
        if (buffer && buffer_size > 0) {
            buffer[0] = '\0';
        }
        return;
    }

    buffer[0] = '\0';

    // Get the property
    CFTypeRef property = IORegistryEntryCreateCFProperty(entry, key, kCFAllocatorDefault, 0);
    if (property) {
        cf_string_to_cstr(property, buffer, buffer_size);
        CFRelease(property); // Always release the CF object
    }
}

// Get a string property from an IOKit registry entry's parent safely
static void get_parent_iokit_string_property(io_registry_entry_t entry, CFStringRef key,
                                             char *buffer, size_t buffer_size,
                                             const io_name_t plane) {
    // Initialize buffer to empty
    if (!buffer || buffer_size == 0 || !entry || !key || !plane) {
        if (buffer && buffer_size > 0) {
            buffer[0] = '\0';
        }
        return;
    }

    buffer[0] = '\0';

    io_registry_entry_t parent;

    // Get parent entry
    kern_return_t result = IORegistryEntryGetParentEntry(entry, plane, &parent);
    if (result == KERN_SUCCESS) {
        get_iokit_string_property(parent, key, buffer, buffer_size);
        IOObjectRelease(parent); // Always release the IO object
    }
}

// Get hardware info from IODeviceTree
static void get_devicetree_info(DAEMON_STATUS_FILE *ds) {
    // Initialize all relevant fields to empty
    if (!ds) {
        return;
    }

    ds->hw.product.name[0] = '\0';
    ds->hw.board.name[0] = '\0';
    ds->hw.sys.vendor[0] = '\0';
    ds->hw.product.family[0] = '\0';

    // Get the device tree
    io_registry_entry_t device_tree = IORegistryEntryFromPath(kIOMasterPortDefault, "IODeviceTree:/");
    if (!device_tree) {
        return;
    }

    // Get model information - only operate if device_tree is valid
    get_iokit_string_property(device_tree, CFSTR("model"), ds->hw.product.name, sizeof(ds->hw.product.name));

    // Get board ID if available
    get_iokit_string_property(device_tree, CFSTR("board-id"), ds->hw.board.name, sizeof(ds->hw.board.name));

    // Look for platform information
    io_registry_entry_t platform = IORegistryEntryFromPath(kIOMasterPortDefault, "IODeviceTree:/platform");
    if (platform) {
        // Platform information can sometimes have manufacturer info
        get_iokit_string_property(platform, CFSTR("manufacturer"), ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));

        // Check compatible property for additional info
        char compatible[256] = {0};
        get_iokit_string_property(platform, CFSTR("compatible"), compatible, sizeof(compatible));

        // Parse for product family
        if (compatible[0]) {
            char *family = strstr(compatible, ",");
            if (family && family < compatible + sizeof(compatible) - 1) {
                family++; // Skip the comma
                safecpy(ds->hw.product.family, family);
                dmi_clean_field(ds->hw.product.family, sizeof(ds->hw.product.family));
            }
        }

        IOObjectRelease(platform);
    }

    IOObjectRelease(device_tree);
}

// Get hardware info from IOPlatformExpertDevice
static void get_platform_expert_info(DAEMON_STATUS_FILE *ds) {
    if (!ds) {
        return;
    }

    // Initialize relevant fields if not already set
    if (ds->hw.sys.vendor[0] == '\0') ds->hw.sys.vendor[0] = '\0';
    if (ds->hw.product.name[0] == '\0') ds->hw.product.name[0] = '\0';
    if (ds->hw.product.version[0] == '\0') ds->hw.product.version[0] = '\0';
    if (ds->hw.board.name[0] == '\0') ds->hw.board.name[0] = '\0';
    if (ds->hw.chassis.type[0] == '\0') ds->hw.chassis.type[0] = '\0';

    io_registry_entry_t platform_expert = IORegistryEntryFromPath(
        kIOMasterPortDefault, "IOService:/IOResources/IOPlatformExpertDevice");

    if (!platform_expert) {
        return;
    }

    // System vendor - almost always "Apple Inc." for Macs
    get_iokit_string_property(platform_expert, CFSTR("manufacturer"),
                              ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));

    // Product name
    get_iokit_string_property(platform_expert, CFSTR("model"),
                              ds->hw.product.name, sizeof(ds->hw.product.name));

    // Model number - can be used as product version
    get_iokit_string_property(platform_expert, CFSTR("model-number"),
                              ds->hw.product.version, sizeof(ds->hw.product.version));

    // Board name sometimes available
    get_iokit_string_property(platform_expert, CFSTR("board-id"),
                              ds->hw.board.name, sizeof(ds->hw.board.name));

    // Hardware UUID can be useful to include
    char uuid_str[64] = {0};
    get_iokit_string_property(platform_expert, CFSTR("IOPlatformUUID"), uuid_str, sizeof(uuid_str));

    // Get device type to determine chassis type
    char device_type[64] = {0};
    get_iokit_string_property(platform_expert, CFSTR("device_type"), device_type, sizeof(device_type));

    // Set chassis type based on device_type safely
    if (device_type[0]) {
        if (strcasestr(device_type, "laptop") || strcasestr(device_type, "book")) {
            safecpy(ds->hw.chassis.type, "9");
        } else if (strcasestr(device_type, "server")) {
            safecpy(ds->hw.chassis.type, "17");
        } else if (strcasestr(device_type, "imac")) {
            safecpy(ds->hw.chassis.type, "13");
        } else if (strcasestr(device_type, "mac")) {
            safecpy(ds->hw.chassis.type, "3");
        }
    }

    // If chassis type not set, guess based on product name
    if (!ds->hw.chassis.type[0] && ds->hw.product.name[0]) {
        if (strcasestr(ds->hw.product.name, "book")) {
            safecpy(ds->hw.chassis.type, "9");
        } else if (strcasestr(ds->hw.product.name, "imac")) {
            safecpy(ds->hw.chassis.type, 0, "13");
        } else if (strcasestr(ds->hw.product.name, "mac") &&
                   strcasestr(ds->hw.product.name, "pro")) {
            safecpy(ds->hw.chassis.type, 0, "3");
        } else if (strcasestr(ds->hw.product.name, "mac") &&
                   strcasestr(ds->hw.product.name, "mini")) {
            safecpy(ds->hw.chassis.type, 0, "35");
        }
    }

    IOObjectRelease(platform_expert);
}

// Get SMC revision and system firmware info
static void get_firmware_info(DAEMON_STATUS_FILE *ds) {
    if (!ds) {
        return;
    }

    // Initialize fields if not already set
    if (ds->hw.bios.version[0] == '\0') ds->hw.bios.version[0] = '\0';
    if (ds->hw.bios.vendor[0] == '\0') ds->hw.bios.vendor[0] = '\0';
    if (ds->hw.bios.date[0] == '\0') ds->hw.bios.date[0] = '\0';

    io_registry_entry_t smc = IOServiceGetMatchingService(
        kIOMasterPortDefault, IOServiceMatching("AppleSMC"));

    if (smc) {
        // SMC revision - can be useful for firmware info
        char smc_version[64] = {0};
        get_iokit_string_property(smc, CFSTR("smc-version"), smc_version, sizeof(smc_version));

        if (smc_version[0]) {
            safecpy(ds->hw.bios.version, smc_version);
            dmi_clean_field(ds->hw.bios.version, sizeof(ds->hw.bios.version));
        }

        IOObjectRelease(smc);
    }

    // Check for BIOS information in IODeviceTree:/rom
    io_registry_entry_t rom = IORegistryEntryFromPath(kIOMasterPortDefault, "IODeviceTree:/rom");
    if (rom) {
        // "version" contains firmware version
        get_iokit_string_property(rom, CFSTR("version"), ds->hw.bios.version, sizeof(ds->hw.bios.version));

        // Apple is the vendor for all Mac firmware
        safecpy(ds->hw.bios.vendor, "Apple");

        // "release-date" can sometimes be found
        get_iokit_string_property(rom, CFSTR("release-date"), ds->hw.bios.date, sizeof(ds->hw.bios.date));

        IOObjectRelease(rom);
    }

    // If we still don't have BIOS version, check system version from sysctl
    if (!ds->hw.bios.version[0]) {
        char firmware_version[256] = {0};
        size_t len = sizeof(firmware_version) - 1;

        if (sysctlbyname("machdep.cpu.brand_string", firmware_version, &len, NULL, 0) == 0) {
            firmware_version[len] = '\0'; // Ensure null termination

            // Extract firmware info if present
            char *firmware_info = strstr(firmware_version, "SMC:");
            if (firmware_info && firmware_info < firmware_version + sizeof(firmware_version) - 1) {
                safecpy(ds->hw.bios.version, firmware_info);
                dmi_clean_field(ds->hw.bios.version, sizeof(ds->hw.bios.version));
            }
        }
    }
}

// Get system hardware information using sysctl
static void get_sysctl_info(DAEMON_STATUS_FILE *ds) {
    if (!ds) {
        return;
    }

    // Get model identifier using sysctl if not already set
    if (!ds->hw.product.name[0]) {
        char model[256] = {0};
        size_t len = sizeof(model) - 1;

        if (sysctlbyname("hw.model", model, &len, NULL, 0) == 0) {
            model[len] = '\0'; // Ensure null termination
            safecpy(ds->hw.product.name, model);
            dmi_clean_field(ds->hw.product.name, sizeof(ds->hw.product.name));

            // If chassis type is still not set, guess from model
            if (!ds->hw.chassis.type[0]) {
                if (strncasecmp(model, "MacBook", 7) == 0) {
                    safecpy(ds->hw.chassis.type, "9");
                } else if (strncasecmp(model, "iMac", 4) == 0) {
                    safecpy(ds->hw.chassis.type, "13");
                } else if (strncasecmp(model, "Mac", 3) == 0 &&
                           strcasestr(model, "Pro") != NULL) {
                    strncpy(ds->hw.chassis.type, "3", sizeof(ds->hw.chassis.type) - 1);
                    ds->hw.chassis.type[sizeof(ds->hw.chassis.type) - 1] = '\0';
                } else if (strncasecmp(model, "Mac", 3) == 0 &&
                           strcasestr(model, "mini") != NULL) {
                    strncpy(ds->hw.chassis.type, "35", sizeof(ds->hw.chassis.type) - 1);
                    ds->hw.chassis.type[sizeof(ds->hw.chassis.type) - 1] = '\0';
                } else {
                    strncpy(ds->hw.chassis.type, "3", sizeof(ds->hw.chassis.type) - 1); // Default to desktop
                    ds->hw.chassis.type[sizeof(ds->hw.chassis.type) - 1] = '\0';
                }
            }
        }
    }

    // Get CPU information if board name not set
    if (!ds->hw.board.name[0]) {
        char cpu_brand[256] = {0};
        size_t len = sizeof(cpu_brand) - 1;

        if (sysctlbyname("machdep.cpu.brand_string", cpu_brand, &len, NULL, 0) == 0) {
            cpu_brand[len] = '\0'; // Ensure null termination
            // Use CPU information as part of board info if not available
            strncpy(ds->hw.board.name, cpu_brand, sizeof(ds->hw.board.name) - 1);
            ds->hw.board.name[sizeof(ds->hw.board.name) - 1] = '\0';
            dmi_clean_field(ds->hw.board.name, sizeof(ds->hw.board.name));
        }
    }
}

// Main function to get hardware info
void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    if (!ds) {
        return;
    }

    // Always set Apple as default vendor
    strncpy(ds->hw.sys.vendor, "Apple", sizeof(ds->hw.sys.vendor) - 1);
    ds->hw.sys.vendor[sizeof(ds->hw.sys.vendor) - 1] = '\0';

    // Get info from IOPlatformExpertDevice
    get_platform_expert_info(ds);

    // Get info from IODeviceTree
    get_devicetree_info(ds);

    // Get firmware information
    get_firmware_info(ds);

    // Get additional info from sysctl
    get_sysctl_info(ds);

    // Set board vendor to match system vendor if not set
    if (!ds->hw.board.vendor[0] && ds->hw.sys.vendor[0]) {
        strncpy(ds->hw.board.vendor, ds->hw.sys.vendor, sizeof(ds->hw.board.vendor) - 1);
        ds->hw.board.vendor[sizeof(ds->hw.board.vendor) - 1] = '\0';
    }

    // Set chassis vendor to match system vendor if not set
    if (!ds->hw.chassis.vendor[0] && ds->hw.sys.vendor[0]) {
        strncpy(ds->hw.chassis.vendor, ds->hw.sys.vendor, sizeof(ds->hw.chassis.vendor) - 1);
        ds->hw.chassis.vendor[sizeof(ds->hw.chassis.vendor) - 1] = '\0';
    }

    // Default product name if all methods failed
    if (!ds->hw.product.name[0]) {
        strncpy(ds->hw.product.name, "Mac", sizeof(ds->hw.product.name) - 1);
        ds->hw.product.name[sizeof(ds->hw.product.name) - 1] = '\0';
    }

    // Default chassis type if we couldn't determine it
    if (!ds->hw.chassis.type[0]) {
        strncpy(ds->hw.chassis.type, "3", sizeof(ds->hw.chassis.type) - 1); // Desktop
        ds->hw.chassis.type[sizeof(ds->hw.chassis.type) - 1] = '\0';
    }
}

#elif defined(OS_FREEBSD)

#include <sys/types.h>
#include <sys/sysctl.h>
#include <kenv.h>

static void freebsd_get_sysctl_str(const char *name, char *dst, size_t dst_size) {
    size_t len = dst_size - 1; // Reserve space for null terminator
    if (sysctlbyname(name, dst, &len, NULL, 0) == 0 && len < dst_size) {
        dst[len] = '\0';  // Ensure null termination
        dmi_clean_field(dst, dst_size);
    } else {
        dst[0] = '\0';
    }
}

static void freebsd_get_kenv_str(const char *name, char *dst, size_t dst_size) {
    if (kenv(KENV_GET, name, dst, dst_size - 1) == -1) {
        dst[0] = '\0';
    } else {
        dst[dst_size - 1] = '\0'; // Force null termination
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

// Helper function to safely read a registry string value
static void windows_read_registry_string(HKEY key_base, const char *subkey_path,
                                         const char *value_name, char *dst, size_t dst_size) {
    HKEY key;
    DWORD type;
    DWORD size = dst_size;

    // Initialize output buffer to empty string
    dst[0] = '\0';

    // Open the registry key
    if (RegOpenKeyExA(key_base, subkey_path, 0, KEY_READ, &key) != ERROR_SUCCESS) {
        return;
    }

    // Read the registry value
    if (RegQueryValueExA(key, value_name, NULL, &type, (LPBYTE)dst, &size) == ERROR_SUCCESS) {
        if ((type == REG_SZ || type == REG_EXPAND_SZ || type == REG_MULTI_SZ) && size > 0) {
            // Ensure data is properly null-terminated
            if (size >= dst_size) {
                size = dst_size - 1;
            }
            dst[size] = '\0';
            dmi_clean_field(dst, dst_size);
        }
    }

    RegCloseKey(key);
}

// Helper to safely parse SMBIOS strings
static char *get_smbios_string(char *start, int index, char *buffer, size_t buffer_size,
                               BYTE *smbios_data, DWORD smbios_size) {
    if (!start || !buffer || buffer_size == 0 || index <= 0) {
        buffer[0] = '\0';
        return NULL;
    }

    // Initialize buffer
    buffer[0] = '\0';

    // Check if start pointer is within bounds
    if (start < (char*)smbios_data || start >= (char*)(smbios_data + smbios_size)) {
        return NULL;
    }

    // Find the indexed string
    char *s = start;
    int i = 1; // SMBIOS strings are 1-based

    // Maximum limit to prevent infinite loops with corrupted data
    const int MAX_ITERATIONS = 100;
    int iterations = 0;

    while (i < index && iterations < MAX_ITERATIONS) {
        // Check if current position is valid
        if (s < (char*)smbios_data || s >= (char*)(smbios_data + smbios_size - 1)) {
            return NULL;
        }

        // Find end of current string
        while (*s != '\0') {
            if (s >= (char*)(smbios_data + smbios_size - 1)) {
                return NULL; // Hit end of buffer
            }
            s++;
        }

        // Move to next string
        s++;
        if (*s == '\0') {
            return NULL; // End of strings section
        }

        i++;
        iterations++;
    }

    // If we found the string and it's not empty
    if (i == index && *s) {
        // Copy safely with bounds checking
        size_t j = 0;
        while (j < buffer_size - 1 && *s) {
            // Ensure we're not reading past the end of smbios_data
            if (s >= (char*)(smbios_data + smbios_size)) {
                break;
            }
            buffer[j++] = *s++;
        }
        buffer[j] = '\0';
        dmi_clean_field(buffer, buffer_size);
        return buffer;
    }

    return NULL;
}

// Helper to get SMBIOS data
static void windows_get_smbios_info(DAEMON_STATUS_FILE *ds) {
    BYTE *smbios_data = NULL;

    // Request SMBIOS data size
    DWORD smbios_size = GetSystemFirmwareTable('RSMB', 0, NULL, 0);
    if (smbios_size == 0 || smbios_size > 1024*1024) { // Sanity check on size
        return;
    }

    // Allocate memory for SMBIOS data
    smbios_data = (BYTE *)malloc(smbios_size);
    if (!smbios_data) {
        return;
    }

    // Get the SMBIOS data
    DWORD result = GetSystemFirmwareTable('RSMB', 0, smbios_data, smbios_size);
    if (result == 0 || result > smbios_size) {
        free(smbios_data);
        return;
    }

    // Temporary buffer for string parsing
    char temp_str[256];

    // The SMBIOS header is at offset 8
    if (smbios_size < 8 + 4) { // Need at least header plus structure header
        free(smbios_data);
        return;
    }

    BYTE *ptr = smbios_data + 8;
    BYTE *end_ptr = smbios_data + smbios_size;

    // Track already visited types to avoid duplicates in corrupted tables
    unsigned char visited_types[256] = {0};

    // Parse the tables
    while (ptr + 4 <= end_ptr) { // Need at least type, length, handle
        BYTE type = ptr[0];
        BYTE length = ptr[1];

        // Sanity checks
        if (length < 4 || ptr + length > end_ptr) {
            break;
        }

        // Skip if we've already processed this type
        if (visited_types[type]) {
            // Find string terminator (double NULL)
            BYTE *str_ptr = ptr + length;
            int found_terminator = 0;

            while (str_ptr + 1 < end_ptr && !found_terminator) {
                if (str_ptr[0] == 0 && str_ptr[1] == 0) {
                    found_terminator = 1;
                }
                str_ptr++;
            }

            if (found_terminator) {
                ptr = str_ptr + 1;
                continue;
            } else {
                break; // Corrupt data
            }
        }

        // Mark this type as visited
        visited_types[type] = 1;

        // Calculate string table offset
        char *string_table = (char*)(ptr + length);
        if (string_table >= (char*)end_ptr) {
            break;
        }

        switch (type) {
            case 0: // BIOS Information
                if (length >= 18) { // Minimum size for BIOS info
                    // BIOS Vendor (string index at offset 4)
                    if (ptr[4] > 0 && get_smbios_string(string_table, ptr[4], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.bios.vendor, temp_str);
                    }

                    // BIOS Version (string index at offset 5)
                    if (ptr[5] > 0 && get_smbios_string(string_table, ptr[5], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.bios.version, temp_str);
                    }

                    // BIOS Release Date (string index at offset 8)
                    if (ptr[8] > 0 && get_smbios_string(string_table, ptr[8], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.bios.date, temp_str);
                    }
                }
                break;

            case 1: // System Information
                if (length >= 8) { // Minimum size for System info
                    // Manufacturer (string index at offset 4)
                    if (ptr[4] > 0 && get_smbios_string(string_table, ptr[4], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.sys.vendor, temp_str);
                    }

                    // Product Name (string index at offset 5)
                    if (ptr[5] > 0 && get_smbios_string(string_table, ptr[5], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.product.name, temp_str);
                    }

                    // Version (string index at offset 6)
                    if (ptr[6] > 0 && get_smbios_string(string_table, ptr[6], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.product.version, temp_str);
                    }

                    // If structure is long enough for family (SMBIOS 2.1+)
                    if (length >= 25 && ptr[21] > 0 && get_smbios_string(string_table, ptr[21], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.product.family, temp_str);
                    }
                }
                break;

            case 2: // Baseboard Information
                if (length >= 8) { // Minimum size for Baseboard info
                    // Manufacturer (string index at offset 4)
                    if (ptr[4] > 0 && get_smbios_string(string_table, ptr[4], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.board.vendor, temp_str);
                    }

                    // Product (string index at offset 5)
                    if (ptr[5] > 0 && get_smbios_string(string_table, ptr[5], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.board.name, temp_str);
                    }

                    // Version (string index at offset 6)
                    if (ptr[6] > 0 && get_smbios_string(string_table, ptr[6], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.board.version, temp_str);
                    }
                }
                break;

            case 3: // Chassis Information
                if (length >= 9) { // Minimum size for Chassis info
                    // Manufacturer (string index at offset 4)
                    if (ptr[4] > 0 && get_smbios_string(string_table, ptr[4], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.chassis.vendor, temp_str);
                    }

                    // Type (numerical value at offset 5)
                    BYTE chassis_type = ptr[5] & 0x7F; // Mask out MSB
                    if (chassis_type > 0 && chassis_type < 36) { // Valid range check
                        snprintf(ds->hw.chassis.type, sizeof(ds->hw.chassis.type), "%d", chassis_type);
                    }

                    // Version (string index at offset 6)
                    if (ptr[6] > 0 && get_smbios_string(string_table, ptr[6], temp_str, sizeof(temp_str), smbios_data, smbios_size)) {
                        safecpy(ds->hw.chassis.version, temp_str);
                    }
                }
                break;
        }

        // Find end of strings section (double null terminator)
        char *str_end = string_table;
        int found_term = 0;

        // Set a reasonable limit to avoid infinite loops with corrupted data
        const int MAX_STRING_SEARCH = 10000;
        int string_search_count = 0;

        while (str_end + 1 < (char*)end_ptr && !found_term && string_search_count < MAX_STRING_SEARCH) {
            if (str_end[0] == 0 && str_end[1] == 0) {
                found_term = 1;
            }
            str_end++;
            string_search_count++;
        }

        if (!found_term) {
            break; // Corrupt data
        }

        // Move to next structure
        ptr = (BYTE*)(str_end + 1);

        // Check if we've reached the end of the table
        if (ptr >= end_ptr || *ptr == 127) {
            break;
        }
    }

    free(smbios_data);
}

// Fallback method using registry
static void windows_get_registry_info(DAEMON_STATUS_FILE *ds) {
    // System manufacturer and model from registry
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "SystemManufacturer",
        ds->hw.sys.vendor,
        sizeof(ds->hw.sys.vendor)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "SystemProductName",
        ds->hw.product.name,
        sizeof(ds->hw.product.name)
    );

    // BIOS information
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BIOSVendor",
        ds->hw.bios.vendor,
        sizeof(ds->hw.bios.vendor)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BIOSVersion",
        ds->hw.bios.version,
        sizeof(ds->hw.bios.version)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BIOSReleaseDate",
        ds->hw.bios.date,
        sizeof(ds->hw.bios.date)
    );

    // Board information
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BaseBoardManufacturer",
        ds->hw.board.vendor,
        sizeof(ds->hw.board.vendor)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BaseBoardProduct",
        ds->hw.board.name,
        sizeof(ds->hw.board.name)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BaseBoardVersion",
        ds->hw.board.version,
        sizeof(ds->hw.board.version)
    );
}

// Main function to get hardware information
void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    // Initialize all strings to empty for safety
    ds->hw.sys.vendor[0] = '\0';
    ds->hw.product.name[0] = '\0';
    ds->hw.product.version[0] = '\0';
    ds->hw.product.sku[0] = '\0';
    ds->hw.product.family[0] = '\0';
    ds->hw.board.vendor[0] = '\0';
    ds->hw.board.name[0] = '\0';
    ds->hw.board.version[0] = '\0';
    ds->hw.bios.vendor[0] = '\0';
    ds->hw.bios.version[0] = '\0';
    ds->hw.bios.date[0] = '\0';
    ds->hw.bios.release[0] = '\0';
    ds->hw.chassis.vendor[0] = '\0';
    ds->hw.chassis.version[0] = '\0';
    ds->hw.chassis.type[0] = '\0';

    // First try SMBIOS data through firmware table API
    windows_get_smbios_info(ds);

    // Try registry as a fallback or to fill missing values
    windows_get_registry_info(ds);

    // If chassis type is not set or not a valid number, set a default
    if (!ds->hw.chassis.type[0] || atoi(ds->hw.chassis.type) <= 0) {
        // Check common system names for laptops
        if (strcasestr(ds->hw.product.name, "notebook") != NULL ||
            strcasestr(ds->hw.product.name, "laptop") != NULL ||
            strcasestr(ds->hw.product.name, "book") != NULL) {
            safecpy(ds->hw.chassis.type, "9");  // Laptop
        }
        // Check for servers
        else if (strcasestr(ds->hw.product.name, "server") != NULL) {
            safecpy(ds->hw.chassis.type, "17");  // Server
        }
        // Default to desktop
        else {
            safecpy(ds->hw.chassis.type, "3");  // Desktop
        }
    }

    // Set defaults for missing values
    if (!ds->hw.sys.vendor[0]) {
        safecpy(ds->hw.sys.vendor, "Unknown");
    }

    if (!ds->hw.product.name[0]) {
        safecpy(ds->hw.product.name, "Unknown");
    }
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
        safecpy(ds->hw.sys.vendor, ds->cloud_provider_type);

    if(ds->cloud_instance_type[0] && strcmp(ds->cloud_instance_type, "unknown") != 0)
        safecpy(ds->hw.product.name, ds->cloud_instance_type);

    if(ds->virtualization[0] && strcmp(ds->virtualization, "none") != 0 && strcmp(ds->virtualization, "unknown") != 0)
        safecpy(ds->hw.chassis.type, "vm");
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
        safecpy(ds->hw.sys.vendor, ds->hw.board.vendor);
    if(!ds->hw.sys.vendor[0])
        safecpy(ds->hw.sys.vendor, ds->hw.chassis.vendor);
    if(!ds->hw.sys.vendor[0])
        safecpy(ds->hw.sys.vendor, ds->hw.bios.vendor);
    if(!ds->hw.sys.vendor[0])
        safecpy(ds->hw.sys.vendor, "Unknown");

    // make sure we have a product name
    if(!ds->hw.product.name[0])
        safecpy(ds->hw.product.name, ds->hw.board.name);
    if(!ds->hw.product.name[0])
        safecpy(ds->hw.product.name, "Unknown");

    // make sure the cloud provider and cloud instance loaded from system-info.sh are preferred
    finalize_vendor_product_vm(ds);
}
