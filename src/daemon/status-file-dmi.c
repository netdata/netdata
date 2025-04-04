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
        {"0123456789", ""},
        {"SKU", ""},
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
        {"QEMU", "KVM"},

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
        {"Google Inc", "Google"},

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

        {"Shenzhen Meigao Electronic Equipment Co.,Ltd", "Meigao"},
        {"Micro Computer (HK) Tech Limited", "Micro Computer"},
        {"Micro Computer(HK) Tech Limited", "Micro Computer"},

        {"MICRO-STAR INTERNATIONAL CO., LTD", "MSI"},
        {"MICRO-STAR INTERNATIONAL CO.,LTD", "MSI"},
        {"MSI", "MSI"},
        {"Micro-Star International Co., Ltd", "MSI"},
        {"Micro-Star International Co., Ltd.", "MSI"},

        {"MICROSOFT", "Microsoft"},
        {"Microsoft Corporation", "Microsoft"},

        {"nVIDIA", "NVIDIA"},

        {"OPENSTACK", "OpenStack"},
        {"OpenStack Foundation", "OpenStack"},

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

        {"SUN MICROSYSTEMS", "Sun"},

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
    // see also inxi: https://github.com/smxi/inxi/blob/master/inxi

    // we categorize chassis types as:
    //   1. desktop
    //   2. laptop
    //   3. server
    //   4. mini-pc
    //   5. unknown
    // elsewhere we mark them also as "vm" (which is preferred over this)

    switch(chassis_type) {
        case 3: /* "desktop" */
        case 4: /* "low-profile-desktop" */
        case 6: /* "mini-tower-desktop" */
        case 7: /* "tower-desktop" */
        case 13: /* "all-in-one" */
        case 15: /* "space-saving-desktop" */
        case 24: /* "sealed-desktop" */
        case 26: /* "compact-pci" */
            return "desktop";

        case 5: /* "pizza-box" - was 1U desktops and some laptops */
        case 8: /* "portable" */
        case 9: /* "laptop" */
        case 10: /* "notebook" */
        case 11: /* "hand-held" */
        case 12: /* "docking-station" */
        case 14: /* "sub-notebook" */
        case 16: /* "lunch-box" */
        case 30: /* "tablet" */
        case 31: /* "convertible" */
        case 32: /* "detachable" */
            return "laptop";

        case 17: /* "main-server-chassis" */
        case 23: /* "rack-mount-server" */
        case 25: /* "multimount-chassis" */
        case 27: /* "advanced-tca" */
        case 28: /* "blade" */
        case 29: /* "blade-enclosure" */
            return "server";

        case 33: /* "iot-gateway" */
        case 34: /* "embedded-pc" */
        case 35: /* "mini-pc" */
        case 36: /* "stick-pc" */
            return "mini-pc";

        case 1: /* "other" */
        case 2: /* "unknown" */
        case 18: /* "expansion-chassis" */
        case 19: /* "sub-chassis" */
        case 20: /* "bus-expansion" */
        case 21: /* "peripheral" */
        case 22: /* "raid" */
        default:
            return "unknown";
    }
}

static void dmi_clean_field(char *buf, size_t buf_size) {
    if(!buf || !buf_size) return;

    // replace non-ascii characters and control characters with spaces
    for (size_t i = 0; i < buf_size && buf[i]; i++) {
        if (!isascii((uint8_t)buf[i]) || iscntrl((uint8_t)buf[i]))
            buf[i] = ' ';
    }

    // detect if all characters are symbols
    bool contains_alnum = false;
    for (size_t i = 0; i < buf_size && buf[i]; i++) {
        if (isalnum((uint8_t)buf[i])) {
            contains_alnum = true;
            break;
        }
    }

    if (!contains_alnum || !buf[0]) {
        buf[0] = '\0';
        return;
    }

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

    char buf[MAX(256, dst_size)];
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
    if (!buffer || buffer_size == 0)
        return;

    buffer[0] = '\0';

    // Safety check for null CF reference
    if (!cf_val)
        return;

    // Pre-set the last byte to ensure termination even if conversion fails
    buffer[buffer_size - 1] = '\0';

    if (CFGetTypeID(cf_val) == CFStringGetTypeID()) {
        CFStringRef str_ref = (CFStringRef)cf_val;
        // Use CFStringGetCString with len-1 to ensure space for null terminator
        if (!CFStringGetCString(str_ref, buffer, buffer_size - 1, kCFStringEncodingUTF8))
            buffer[0] = '\0'; // Reset on failure
    }
    else if (CFGetTypeID(cf_val) == CFDataGetTypeID()) {
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
static void get_iokit_string_property(io_registry_entry_t entry, CFStringRef key, char *buffer, size_t buffer_size) {
    // Initialize buffer to empty
    if (!buffer || buffer_size == 0 || !entry || !key) {
        if (buffer && buffer_size > 0)
            buffer[0] = '\0';

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
        if (buffer && buffer_size > 0)
            buffer[0] = '\0';

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
    if (!ds)
        return;

    ds->hw.product.name[0] = '\0';
    ds->hw.board.name[0] = '\0';
    ds->hw.sys.vendor[0] = '\0';
    ds->hw.product.family[0] = '\0';

    // Get the device tree
    io_registry_entry_t device_tree = IORegistryEntryFromPath(kIOMasterPortDefault, "IODeviceTree:/");
    if (!device_tree)
        return;

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
    if (!ds)
        return;

    io_registry_entry_t platform_expert = IORegistryEntryFromPath(
        kIOMasterPortDefault, "IOService:/IOResources/IOPlatformExpertDevice");

    if (!platform_expert)
        return;

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
        if (strcasestr(device_type, "laptop") || strcasestr(device_type, "book"))
            safecpy(ds->hw.chassis.type, "9");
        else if (strcasestr(device_type, "server"))
            safecpy(ds->hw.chassis.type, "17");
        else if (strcasestr(device_type, "imac"))
            safecpy(ds->hw.chassis.type, "13");
        else if (strcasestr(device_type, "mac"))
            safecpy(ds->hw.chassis.type, "3");
    }

    // If chassis type not set, guess based on product name
    if (!ds->hw.chassis.type[0] && ds->hw.product.name[0]) {
        if (strcasestr(ds->hw.product.name, "book"))
            safecpy(ds->hw.chassis.type, "9");
        else if (strcasestr(ds->hw.product.name, "imac"))
            safecpy(ds->hw.chassis.type, "13");
        else if (strcasestr(ds->hw.product.name, "mac") && strcasestr(ds->hw.product.name, "pro"))
            safecpy(ds->hw.chassis.type, "3");
        else if (strcasestr(ds->hw.product.name, "mac") && strcasestr(ds->hw.product.name, "mini"))
            safecpy(ds->hw.chassis.type, "35");
    }

    IOObjectRelease(platform_expert);
}

// Get SMC revision and system firmware info
static void get_firmware_info(DAEMON_STATUS_FILE *ds) {
    if (!ds)
        return;

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
    if (!ds)
        return;

    // Get model identifier using sysctl if not already set
    if (!ds->hw.product.name[0]) {
        char model[256] = { 0 };
        size_t len = sizeof(model) - 1;

        if (sysctlbyname("hw.model", model, &len, NULL, 0) == 0) {
            model[len] = '\0';
            safecpy(ds->hw.product.name, model);
            dmi_clean_field(ds->hw.product.name, sizeof(ds->hw.product.name));

            // If chassis type is still not set, guess from model
            if (!ds->hw.chassis.type[0]) {
                if (strncasecmp(model, "MacBook", 7) == 0)
                    safecpy(ds->hw.chassis.type, "9");
                else if (strncasecmp(model, "iMac", 4) == 0)
                    safecpy(ds->hw.chassis.type, "13");
                else if (strncasecmp(model, "Mac", 3) == 0 && strcasestr(model, "Pro") != NULL)
                    safecpy(ds->hw.chassis.type, "3");
                else if (strncasecmp(model, "Mac", 3) == 0 && strcasestr(model, "mini") != NULL)
                    safecpy(ds->hw.chassis.type, "35");
                else
                    safecpy(ds->hw.chassis.type, "3"); // Default to desktop
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
            safecpy(ds->hw.board.name, cpu_brand);
            dmi_clean_field(ds->hw.board.name, sizeof(ds->hw.board.name));
        }
    }
}

// Main function to get hardware info
void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    if (!ds) return;

    // Always set Apple as default vendor
    safecpy(ds->hw.sys.vendor, "Apple");

    // Get info from IOPlatformExpertDevice
    get_platform_expert_info(ds);

    // Get info from IODeviceTree
    get_devicetree_info(ds);

    // Get firmware information
    get_firmware_info(ds);

    // Get additional info from sysctl
    get_sysctl_info(ds);

    // Set board vendor to match system vendor if not set
    if (!ds->hw.board.vendor[0] && ds->hw.sys.vendor[0])
        safecpy(ds->hw.board.vendor, ds->hw.sys.vendor);

    // Set chassis vendor to match system vendor if not set
    if (!ds->hw.chassis.vendor[0] && ds->hw.sys.vendor[0])
        safecpy(ds->hw.chassis.vendor, ds->hw.sys.vendor);

    // Set bios vendor to match system vendor if not set
    if (!ds->hw.bios.vendor[0] && ds->hw.sys.vendor[0])
        safecpy(ds->hw.bios.vendor, ds->hw.sys.vendor);

    // Default product name if all methods failed
    if (!ds->hw.product.name[0])
        safecpy(ds->hw.product.name, "Mac");

    // Default chassis type if we couldn't determine it
    if (!ds->hw.chassis.type[0])
        safecpy(ds->hw.chassis.type, "3"); // Desktop
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
    }
    else
        dst[0] = '\0';
}

static void freebsd_get_kenv_str(const char *name, char *dst, size_t dst_size) {
    dst[0] = '\0';
    if (kenv(KENV_GET, name, dst, dst_size - 1) == -1)
        dst[0] = '\0';
    else {
        dst[dst_size - 1] = '\0';
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
    freebsd_get_kenv_str("smbios.chassis.type", ds->hw.chassis.type, sizeof(ds->hw.chassis.type));

    // If we couldn't get system information from SMBIOS, try to use model
    if (!ds->hw.product.name[0])
        freebsd_get_sysctl_str("hw.model", ds->hw.product.name, sizeof(ds->hw.product.name));
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
    if (RegOpenKeyExA(key_base, subkey_path, 0, KEY_READ, &key) != ERROR_SUCCESS)
        return;

    // Read the registry value
    if (RegQueryValueExA(key, value_name, NULL, &type, (LPBYTE)dst, &size) == ERROR_SUCCESS) {
        if ((type == REG_SZ || type == REG_EXPAND_SZ || type == REG_MULTI_SZ) && size > 0) {

            if (size >= dst_size)
                size = dst_size - 1;

            dst[size] = '\0';
            dmi_clean_field(dst, dst_size);
        }
    }

    RegCloseKey(key);
}

// SMBIOS structure header definition
typedef struct {
    uint8_t type;
    uint8_t length;
    uint16_t handle;
    // Remaining fields are variable
} smbios_header_t;

// SMBIOS data container
typedef struct {
    BYTE *data;           // The raw SMBIOS data buffer
    DWORD size;           // Size of the data buffer
    DWORD entries_count;  // Number of valid structure entries found
    bool valid;           // Whether the data is valid
} smbios_data_t;

// Helper to safely parse SMBIOS strings with improved bounds checking
static char *get_smbios_string(const BYTE *table_start, DWORD table_size, 
                             const char *string_start, uint8_t index, 
                             char *buffer, size_t buffer_size) {
    if (!string_start || !buffer || buffer_size == 0 || index == 0) {
        if (buffer && buffer_size > 0)
            buffer[0] = '\0';
        return NULL;
    }
    
    buffer[0] = '\0';
    
    // Check if string_start is within bounds of the table
    if (string_start < (const char*)table_start || 
        string_start >= (const char*)(table_start + table_size)) {
        return NULL;
    }
    
    // Navigate to the indexed string
    const char *s = string_start;
    const char *end = (const char*)(table_start + table_size);
    uint8_t current_index = 1;
    
    // Set a reasonable limit to prevent infinite loops with corrupted data
    const int MAX_ITERATIONS = 100;
    int iterations = 0;
    
    while (current_index < index && s < end && iterations < MAX_ITERATIONS) {
        // Skip to end of current string
        while (s < end && *s != '\0')
            s++;
            
        // Skip past terminating null
        if (s < end)
            s++;
            
        // If we hit the end of strings section (double null) or table boundary
        if (s >= end || *s == '\0')
            return NULL;
            
        current_index++;
        iterations++;
    }
    
    // If we found our string
    if (current_index == index && s < end && *s != '\0') {
        // Copy safely with explicit bounds checking
        size_t i = 0;
        while (s < end && *s != '\0' && i < buffer_size - 1) {
            buffer[i++] = *s++;
        }
        buffer[i] = '\0';
        
        dmi_clean_field(buffer, buffer_size);
        return buffer;
    }
    
    return NULL;
}

// Get SMBIOS data with proper memory management
static smbios_data_t get_smbios_data(void) {
    smbios_data_t result = {NULL, 0, 0, false};
    
    // Request SMBIOS data size
    DWORD size = GetSystemFirmwareTable('RSMB', 0, NULL, 0);
    if (size == 0 || size > 1024*1024) // Sanity check on size
        return result;
        
    // Allocate memory with zero initialization
    result.data = (BYTE *)mallocz(size);
    if (!result.data)
        return result;
        
    result.size = size;
    
    // Get the SMBIOS data
    DWORD bytes_read = GetSystemFirmwareTable('RSMB', 0, result.data, result.size);
    if (bytes_read == 0 || bytes_read > result.size) {
        freez(result.data);
        result.data = NULL;
        result.size = 0;
        return result;
    }
    
    result.valid = true;
    return result;
}

// Process BIOS Information (Type 0)
static void process_smbios_bios_info(const smbios_header_t *header,
                                  DAEMON_STATUS_FILE *ds,
                                  const char *string_table,
                                  const BYTE *smbios_data,
                                  DWORD smbios_size) {
    if (header->length < 18) // Minimum size for BIOS info
        return;
        
    const BYTE *data = (const BYTE *)header;
    char temp_str[256];
    
    // BIOS Vendor (string index at offset 4)
    if (data[4] > 0 && get_smbios_string(smbios_data, smbios_size, string_table, 
                                     data[4], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.bios.vendor, temp_str);
    }
    
    // BIOS Version (string index at offset 5)
    if (data[5] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[5], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.bios.version, temp_str);
    }
    
    // BIOS Release Date (string index at offset 8)
    if (data[8] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[8], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.bios.date, temp_str);
    }
}

// Process System Information (Type 1)
static void process_smbios_system_info(const smbios_header_t *header,
                                    DAEMON_STATUS_FILE *ds,
                                    const char *string_table,
                                    const BYTE *smbios_data,
                                    DWORD smbios_size) {
    if (header->length < 8) // Minimum size for System info
        return;
        
    const BYTE *data = (const BYTE *)header;
    char temp_str[256];
    
    // Manufacturer (string index at offset 4)
    if (data[4] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[4], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.sys.vendor, temp_str);
    }
    
    // Product Name (string index at offset 5)
    if (data[5] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[5], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.product.name, temp_str);
    }
    
    // Version (string index at offset 6)
    if (data[6] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[6], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.product.version, temp_str);
    }
    
    // If structure is long enough for family (SMBIOS 2.1+)
    if (header->length >= 25 && data[21] > 0 && 
        get_smbios_string(smbios_data, smbios_size, string_table,
                       data[21], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.product.family, temp_str);
    }
}

// Process Baseboard Information (Type 2)
static void process_smbios_baseboard_info(const smbios_header_t *header,
                                       DAEMON_STATUS_FILE *ds,
                                       const char *string_table,
                                       const BYTE *smbios_data,
                                       DWORD smbios_size) {
    if (header->length < 8) // Minimum size for Baseboard info
        return;
        
    const BYTE *data = (const BYTE *)header;
    char temp_str[256];
    
    // Manufacturer (string index at offset 4)
    if (data[4] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[4], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.board.vendor, temp_str);
    }
    
    // Product (string index at offset 5)
    if (data[5] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[5], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.board.name, temp_str);
    }
    
    // Version (string index at offset 6)
    if (data[6] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[6], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.board.version, temp_str);
    }
}

// Process Chassis Information (Type 3)
static void process_smbios_chassis_info(const smbios_header_t *header,
                                     DAEMON_STATUS_FILE *ds,
                                     const char *string_table,
                                     const BYTE *smbios_data,
                                     DWORD smbios_size) {
    if (header->length < 9) // Minimum size for Chassis info
        return;
        
    const BYTE *data = (const BYTE *)header;
    char temp_str[256];
    
    // Manufacturer (string index at offset 4)
    if (data[4] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[4], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.chassis.vendor, temp_str);
    }
    
    // Type (numerical value at offset 5)
    BYTE chassis_type = data[5] & 0x7F; // Mask out MSB
    snprintf(ds->hw.chassis.type, sizeof(ds->hw.chassis.type), "%d", chassis_type);
    
    // Version (string index at offset 6)
    if (data[6] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[6], temp_str, sizeof(temp_str))) {
        safecpy(ds->hw.chassis.version, temp_str);
    }
}

// Process a complete SMBIOS structure
static void process_smbios_structure(const smbios_header_t *header, 
                                  DAEMON_STATUS_FILE *ds,
                                  const char *string_table,
                                  const BYTE *smbios_data,
                                  DWORD smbios_size) {
    switch (header->type) {
        case 0: // BIOS Information
            process_smbios_bios_info(header, ds, string_table, smbios_data, smbios_size);
            break;
            
        case 1: // System Information
            process_smbios_system_info(header, ds, string_table, smbios_data, smbios_size);
            break;
            
        case 2: // Baseboard Information
            process_smbios_baseboard_info(header, ds, string_table, smbios_data, smbios_size);
            break;
            
        case 3: // Chassis Information
            process_smbios_chassis_info(header, ds, string_table, smbios_data, smbios_size);
            break;
    }
}

// Parse all SMBIOS structures with robust error handling
static void parse_smbios_structures(const smbios_data_t smbios, DAEMON_STATUS_FILE *ds) {
    if (!smbios.valid || !smbios.data || smbios.size < 8)
        return;
        
    // The SMBIOS header is at offset 8
    const BYTE *current = smbios.data + 8;
    const BYTE *end = smbios.data + smbios.size;
    
    // Track already visited types to avoid duplicates in corrupted tables
    uint8_t visited[256] = {0};
    int structures_parsed = 0;
    
    // Temporary buffer for string parsing
    char temp_str[256];
    
    while (current + sizeof(smbios_header_t) <= end) {
        const smbios_header_t *header = (const smbios_header_t *)current;
        
        // Basic sanity checks
        if (header->length < sizeof(smbios_header_t) || current + header->length > end) {
            break; // Invalid structure
        }
        
        // If we've already seen this type and want to skip duplicates
        if (visited[header->type]) {
            // Find the string terminator (double NULL)
            const char *strings = (const char *)(current + header->length);
            const char *str_end = strings;
            bool found_terminator = false;
            
            // Set a reasonable limit to prevent infinite loops
            const int MAX_STRING_SEARCH = 1000;
            int search_count = 0;
            
            // Search with explicit bounds check for string terminator
            while (str_end + 1 < (const char *)end && !found_terminator && search_count < MAX_STRING_SEARCH) {
                if (str_end[0] == 0 && str_end[1] == 0) {
                    found_terminator = true;
                    break;
                }
                str_end++;
                search_count++;
            }
            
            if (found_terminator) {
                // Advance to next structure
                current = (const BYTE *)(str_end + 1);
                continue;
            } else {
                break; // Corrupt data
            }
        }
        
        // Mark this type as processed
        visited[header->type] = 1;
        structures_parsed++;
        
        // Process the structure based on type
        const char *string_table = (const char *)(current + header->length);
        if (string_table < (const char *)end) {
            process_smbios_structure(header, ds, string_table, smbios.data, smbios.size);
        }
        
        // Find string table end (double null terminator)
        const char *str_end = string_table;
        bool found_terminator = false;
        
        // Set a reasonable limit to prevent infinite loops
        const int MAX_STRING_SEARCH = 1000;
        int search_count = 0;
        
        // Search for end of strings with explicit bounds check
        const char *end_char = (const char *)end;
        while (str_end + 1 < end_char && !found_terminator && search_count < MAX_STRING_SEARCH) {
            if (str_end[0] == 0 && str_end[1] == 0) {
                found_terminator = true;
                break;
            }
            str_end++;
            search_count++;
        }
        
        if (!found_terminator) {
            break; // Corrupt data
        }
        
        // Move to the next structure
        current = (const BYTE *)(str_end + 1);
        
        // Check for end marker or out of bounds
        if (current >= end || *current == 127)
            break;
    }
}
static void windows_get_smbios_info(DAEMON_STATUS_FILE *ds) {
    // Get SMBIOS data using our improved container structure
    smbios_data_t smbios = get_smbios_data();
    if (!smbios.valid)
        return;
        
    // Process the SMBIOS data with our improved parser
    parse_smbios_structures(smbios, ds);
    
    // Clean up
    freez(smbios.data);
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
}

#else
void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    ;
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// public API

void fill_dmi_info(DAEMON_STATUS_FILE *ds) {
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

    os_dmi_info(ds);

    product_name_vendor_type(ds);
}

void product_name_vendor_type(DAEMON_STATUS_FILE *ds) {
    char *force_type = NULL;

    if(ds->cloud_provider_type[0] && strcasecmp(ds->cloud_provider_type, "unknown") != 0)
        safecpy(ds->product.vendor, ds->cloud_provider_type);
    else {
        // copy one of the names found in DMI
        if(ds->hw.sys.vendor[0])
            safecpy(ds->product.vendor, ds->hw.sys.vendor);
        else if(ds->hw.board.vendor[0])
            safecpy(ds->product.vendor, ds->hw.board.vendor);
        else if(ds->hw.chassis.vendor[0])
            safecpy(ds->product.vendor, ds->hw.chassis.vendor);
        else if(ds->hw.bios.vendor[0])
            safecpy(ds->product.vendor, ds->hw.bios.vendor);

        // derive the vendor from other DMI fields
        if(!ds->product.vendor[0]) {
            if(strcasestr(ds->hw.product.name, "VirtualMac") != NULL ||
                (strcasestr(ds->hw.board.name, "Apple") != NULL &&
                 strcasestr(ds->hw.board.name, "Virtual") != NULL)) {
                safecpy(ds->product.vendor, "Apple");
                force_type = "vm";
            }
            else if(strcasestr(ds->hw.product.name, "NVIDIA") != NULL &&
                     strcasestr(ds->hw.product.name, "Kit") != NULL) {
                safecpy(ds->product.vendor, "NVIDIA");
                force_type = "vm";
            }
            else if(strcasestr(ds->hw.product.name, "Raspberry") != NULL) {
                safecpy(ds->product.vendor, "Raspberry");
                force_type = "mini-pc";
            }
            else if(strcasestr(ds->hw.product.name, "ODROID") != NULL) {
                safecpy(ds->product.vendor, "Odroid");
                force_type = "mini-pc";
            }
            else if(strcasestr(ds->hw.product.name, "BananaPi") != NULL ||
                     strcasestr(ds->hw.product.name, "Banana Pi") != NULL) {
                safecpy(ds->product.vendor, "BananaPi");
                force_type = "mini-pc";
            }
            else if(strcasestr(ds->hw.product.name, "OrangePi") != NULL ||
                     strcasestr(ds->hw.product.name, "Orange Pi") != NULL) {
                safecpy(ds->product.vendor, "OrangePi");
                force_type = "mini-pc";
            }
        }

        if(!ds->product.vendor[0])
            safecpy(ds->product.vendor, "unknown");
        else
            dmi_normalize_vendor_field(ds->product.vendor, sizeof(ds->product.vendor));
    }

    if(ds->cloud_instance_type[0] && strcasecmp(ds->cloud_instance_type, "unknown") != 0)
        safecpy(ds->product.name, ds->cloud_instance_type);
    else {
        if(ds->hw.product.name[0])
            safecpy(ds->product.name, ds->hw.product.name);
        else if(ds->hw.board.name[0])
            safecpy(ds->product.name, ds->hw.board.name);
        else
            safecpy(ds->product.name, "unknown");
    }

    if(ds->virtualization[0] && strcasecmp(ds->virtualization, "none") != 0 && strcasecmp(ds->virtualization, "unknown") != 0)
        safecpy(ds->product.type, "vm");
    else if(force_type)
        safecpy(ds->product.type, force_type);
    else if(dmi_is_virtual_machine(ds))
        safecpy(ds->product.type, "vm");
    else {
        char *end = NULL;
        int type = (int)strtol(ds->hw.chassis.type, &end, 10);
        if(type && (!end || !*end))
            safecpy(ds->product.type, dmi_chassis_type_to_string(type));
        else
            safecpy(ds->product.type, "unknown");
    }
}
