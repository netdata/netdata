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

void os_dmi_info_get(DMI_INFO *dmi) {
    if (!dmi) return;
    
    linux_get_dmi_field("sys_vendor", NULL, dmi->sys.vendor, sizeof(dmi->sys.vendor));
    linux_get_dmi_field("system_serial", NULL, dmi->sys.serial, sizeof(dmi->sys.serial));
    linux_get_dmi_field("product_uuid", NULL, dmi->sys.uuid, sizeof(dmi->sys.uuid));
    linux_get_dmi_field("chassis_asset_tag", NULL, dmi->sys.asset_tag, sizeof(dmi->sys.asset_tag));

    linux_get_dmi_field("product_name", "/proc/device-tree/model", dmi->product.name, sizeof(dmi->product.name));
    linux_get_dmi_field("product_version", NULL, dmi->product.version, sizeof(dmi->product.version));
    linux_get_dmi_field("product_sku", NULL, dmi->product.sku, sizeof(dmi->product.sku));
    linux_get_dmi_field("product_family", NULL, dmi->product.family, sizeof(dmi->product.family));

    linux_get_dmi_field("chassis_vendor", NULL, dmi->chassis.vendor, sizeof(dmi->chassis.vendor));
    linux_get_dmi_field("chassis_version", NULL, dmi->chassis.version, sizeof(dmi->chassis.version));
    linux_get_dmi_field("chassis_serial", NULL, dmi->chassis.serial, sizeof(dmi->chassis.serial));
    linux_get_dmi_field("chassis_asset_tag", NULL, dmi->chassis.asset_tag, sizeof(dmi->chassis.asset_tag));

    linux_get_dmi_field("board_vendor", NULL, dmi->board.vendor, sizeof(dmi->board.vendor));
    linux_get_dmi_field("board_name", NULL, dmi->board.name, sizeof(dmi->board.name));
    linux_get_dmi_field("board_version", NULL, dmi->board.version, sizeof(dmi->board.version));
    linux_get_dmi_field("board_serial", NULL, dmi->board.serial, sizeof(dmi->board.serial));
    linux_get_dmi_field("board_asset_tag", NULL, dmi->board.asset_tag, sizeof(dmi->board.asset_tag));

    linux_get_dmi_field("bios_vendor", NULL, dmi->bios.vendor, sizeof(dmi->bios.vendor));
    linux_get_dmi_field("bios_version", NULL, dmi->bios.version, sizeof(dmi->bios.version));
    linux_get_dmi_field("bios_date", NULL, dmi->bios.date, sizeof(dmi->bios.date));
    linux_get_dmi_field("bios_release", NULL, dmi->bios.release, sizeof(dmi->bios.release));

    linux_get_dmi_field("chassis_type", NULL, dmi->chassis.type, sizeof(dmi->chassis.type));
    
    // Check if running in UEFI mode - just file existence, no external command
    bool is_uefi = access("/sys/firmware/efi", F_OK) == 0;
    if (is_uefi) {
        safecpy(dmi->bios.mode, "UEFI");
        
        // Check EFI variable for secure boot - direct file access, no external command
        int fd = open("/sys/firmware/efi/efivars/SecureBoot-8be4df61-93ca-11d2-aa0d-00e098032b8c", O_RDONLY);
        if (fd != -1) {
            // Skip the first 4 bytes which contain the EFI variable attributes
            unsigned char value[5] = {0};
            ssize_t bytes_read = read(fd, value, sizeof(value));
            if (bytes_read == sizeof(value)) {
                // The 5th byte (index 4) contains the secure boot status
                dmi->bios.secure_boot = (value[4] == 1);
            }
            close(fd);
        }
        
        // Alternative check through securelevel - direct file access, no external command
        if (!dmi->bios.secure_boot) {
            fd = open("/sys/kernel/security/securelevel", O_RDONLY);
            if (fd != -1) {
                char level[10] = {0};
                ssize_t bytes_read = read(fd, level, sizeof(level) - 1);
                if (bytes_read > 0) {
                    level[bytes_read] = '\0'; // Ensure null termination
                    // If securelevel is > 0, usually means secure boot is enabled
                    int securelevel = atoi(level);
                    dmi->bios.secure_boot = (securelevel > 0);
                }
                close(fd);
            }
        }
    } else {
        safecpy(dmi->bios.mode, "Legacy");
    }
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
static void get_devicetree_info(DMI_INFO *dmi) {
    // Initialize all relevant fields to empty
    if (!dmi)
        return;

    dmi->product.name[0] = '\0';
    dmi->board.name[0] = '\0';
    dmi->sys.vendor[0] = '\0';
    dmi->product.family[0] = '\0';

    // Get the device tree
    io_registry_entry_t device_tree = IORegistryEntryFromPath(kIOMasterPortDefault, "IODeviceTree:/");
    if (!device_tree)
        return;

    // Get model information - only operate if device_tree is valid
    get_iokit_string_property(device_tree, CFSTR("model"), dmi->product.name, sizeof(dmi->product.name));

    // Get board ID if available
    get_iokit_string_property(device_tree, CFSTR("board-id"), dmi->board.name, sizeof(dmi->board.name));

    // Look for platform information
    io_registry_entry_t platform = IORegistryEntryFromPath(kIOMasterPortDefault, "IODeviceTree:/platform");
    if (platform) {
        // Platform information can sometimes have manufacturer info
        get_iokit_string_property(platform, CFSTR("manufacturer"), dmi->sys.vendor, sizeof(dmi->sys.vendor));

        // Check compatible property for additional info
        char compatible[256] = {0};
        get_iokit_string_property(platform, CFSTR("compatible"), compatible, sizeof(compatible));

        // Parse for product family
        if (compatible[0]) {
            char *family = strstr(compatible, ",");
            if (family && family < compatible + sizeof(compatible) - 1) {
                family++; // Skip the comma
                safecpy(dmi->product.family, family);
                dmi_clean_field(dmi->product.family, sizeof(dmi->product.family));
            }
        }

        IOObjectRelease(platform);
    }

    IOObjectRelease(device_tree);
}

// Get hardware info from IOPlatformExpertDevice
static void get_platform_expert_info(DMI_INFO *dmi) {
    if (!dmi)
        return;

    io_registry_entry_t platform_expert = IORegistryEntryFromPath(
        kIOMasterPortDefault, "IOService:/IOResources/IOPlatformExpertDevice");

    if (!platform_expert)
        return;

    // System vendor - almost always "Apple Inc." for Macs
    get_iokit_string_property(platform_expert, CFSTR("manufacturer"),
                              dmi->sys.vendor, sizeof(dmi->sys.vendor));

    // Product name
    get_iokit_string_property(platform_expert, CFSTR("model"),
                              dmi->product.name, sizeof(dmi->product.name));

    // Model number - can be used as product version
    get_iokit_string_property(platform_expert, CFSTR("model-number"),
                              dmi->product.version, sizeof(dmi->product.version));

    // Board name sometimes available
    get_iokit_string_property(platform_expert, CFSTR("board-id"),
                              dmi->board.name, sizeof(dmi->board.name));

    // System serial number - might be available as IOPlatformSerialNumber
    get_iokit_string_property(platform_expert, CFSTR("IOPlatformSerialNumber"), 
                             dmi->sys.serial, sizeof(dmi->sys.serial));

    // Hardware UUID can be useful to include
    char uuid_str[64] = {0};
    get_iokit_string_property(platform_expert, CFSTR("IOPlatformUUID"), uuid_str, sizeof(uuid_str));
    
    if (uuid_str[0])
        safecpy(dmi->sys.uuid, uuid_str);
        
    // For asset tag, try alternative properties since Apple doesn't provide a specific asset tag property
    // First try chassis tag property if available
    char asset_tag[64] = {0};
    get_iokit_string_property(platform_expert, CFSTR("IOPlatformChassisTag"), asset_tag, sizeof(asset_tag));
    
    // If not available, try using the serial number as an asset tag if not already set
    if (!asset_tag[0]) {
        // Model identifier can be a reasonable alternative for asset tag
        get_iokit_string_property(platform_expert, CFSTR("model"), asset_tag, sizeof(asset_tag));
    }
    
    if (asset_tag[0] && (!dmi->sys.serial[0] || strcmp(asset_tag, dmi->sys.serial) != 0)) {
        safecpy(dmi->sys.asset_tag, asset_tag);
    }

    // Get device type to determine chassis type
    char device_type[64] = {0};
    get_iokit_string_property(platform_expert, CFSTR("device_type"), device_type, sizeof(device_type));

    // Set chassis type based on device_type safely
    if (device_type[0]) {
        if (strcasestr(device_type, "laptop") || strcasestr(device_type, "book"))
            safecpy(dmi->chassis.type, "9");
        else if (strcasestr(device_type, "server"))
            safecpy(dmi->chassis.type, "17");
        else if (strcasestr(device_type, "imac"))
            safecpy(dmi->chassis.type, "13");
        else if (strcasestr(device_type, "mac"))
            safecpy(dmi->chassis.type, "3");
    }

    // If chassis type not set, guess based on product name
    if (!dmi->chassis.type[0] && dmi->product.name[0]) {
        if (strcasestr(dmi->product.name, "book"))
            safecpy(dmi->chassis.type, "9");
        else if (strcasestr(dmi->product.name, "imac"))
            safecpy(dmi->chassis.type, "13");
        else if (strcasestr(dmi->product.name, "mac") && strcasestr(dmi->product.name, "pro"))
            safecpy(dmi->chassis.type, "3");
        else if (strcasestr(dmi->product.name, "mac") && strcasestr(dmi->product.name, "mini"))
            safecpy(dmi->chassis.type, "35");
    }

    IOObjectRelease(platform_expert);
}

// Get SMC revision and system firmware info
static void get_firmware_info(DMI_INFO *dmi) {
    if (!dmi)
        return;

    io_registry_entry_t smc = IOServiceGetMatchingService(
        kIOMasterPortDefault, IOServiceMatching("AppleSMC"));

    if (smc) {
        // SMC revision - can be useful for firmware info
        char smc_version[64] = {0};
        get_iokit_string_property(smc, CFSTR("smc-version"), smc_version, sizeof(smc_version));

        if (smc_version[0]) {
            safecpy(dmi->bios.version, smc_version);
            dmi_clean_field(dmi->bios.version, sizeof(dmi->bios.version));
        }

        IOObjectRelease(smc);
    }

    // Check for BIOS information in IODeviceTree:/rom
    io_registry_entry_t rom = IORegistryEntryFromPath(kIOMasterPortDefault, "IODeviceTree:/rom");
    if (rom) {
        // "version" contains firmware version
        get_iokit_string_property(rom, CFSTR("version"), dmi->bios.version, sizeof(dmi->bios.version));

        // Apple is the vendor for all Mac firmware
        safecpy(dmi->bios.vendor, "Apple");

        // "release-date" can sometimes be found
        get_iokit_string_property(rom, CFSTR("release-date"), dmi->bios.date, sizeof(dmi->bios.date));

        IOObjectRelease(rom);
    }

    // If we still don't have BIOS version, check system version from sysctl
    if (!dmi->bios.version[0]) {
        char firmware_version[256] = {0};
        size_t len = sizeof(firmware_version) - 1;

        if (sysctlbyname("machdep.cpu.brand_string", firmware_version, &len, NULL, 0) == 0) {
            firmware_version[len] = '\0'; // Ensure null termination

            // Extract firmware info if present
            char *firmware_info = strstr(firmware_version, "SMC:");
            if (firmware_info && firmware_info < firmware_version + sizeof(firmware_version) - 1) {
                safecpy(dmi->bios.version, firmware_info);
                dmi_clean_field(dmi->bios.version, sizeof(dmi->bios.version));
            }
        }
    }
}

// Get system hardware information using sysctl
static void get_sysctl_info(DMI_INFO *dmi) {
    if (!dmi)
        return;

    // Get model identifier using sysctl if not already set
    if (!dmi->product.name[0]) {
        char model[256] = { 0 };
        size_t len = sizeof(model) - 1;

        if (sysctlbyname("hw.model", model, &len, NULL, 0) == 0) {
            model[len] = '\0';
            safecpy(dmi->product.name, model);
            dmi_clean_field(dmi->product.name, sizeof(dmi->product.name));

            // If chassis type is still not set, guess from model
            if (!dmi->chassis.type[0]) {
                if (strncasecmp(model, "MacBook", 7) == 0)
                    safecpy(dmi->chassis.type, "9");
                else if (strncasecmp(model, "iMac", 4) == 0)
                    safecpy(dmi->chassis.type, "13");
                else if (strncasecmp(model, "Mac", 3) == 0 && strcasestr(model, "Pro") != NULL)
                    safecpy(dmi->chassis.type, "3");
                else if (strncasecmp(model, "Mac", 3) == 0 && strcasestr(model, "mini") != NULL)
                    safecpy(dmi->chassis.type, "35");
                else
                    safecpy(dmi->chassis.type, "3"); // Default to desktop
            }
        }
    }

    // Get CPU information if board name not set
    if (!dmi->board.name[0]) {
        char cpu_brand[256] = {0};
        size_t len = sizeof(cpu_brand) - 1;

        if (sysctlbyname("machdep.cpu.brand_string", cpu_brand, &len, NULL, 0) == 0) {
            cpu_brand[len] = '\0'; // Ensure null termination
            // Use CPU information as part of board info if not available
            safecpy(dmi->board.name, cpu_brand);
            dmi_clean_field(dmi->board.name, sizeof(dmi->board.name));
        }
    }
}

// Main function to get hardware info
void os_dmi_info_get(DMI_INFO *dmi) {
    if (!dmi) return;

    // Always set Apple as default vendor
    safecpy(dmi->sys.vendor, "Apple");

    // Get info from IOPlatformExpertDevice
    get_platform_expert_info(dmi);

    // Get info from IODeviceTree
    get_devicetree_info(dmi);

    // Get firmware information
    get_firmware_info(dmi);

    // Get additional info from sysctl
    get_sysctl_info(dmi);

    // Set board vendor to match system vendor if not set
    if (!dmi->board.vendor[0] && dmi->sys.vendor[0])
        safecpy(dmi->board.vendor, dmi->sys.vendor);

    // Set chassis vendor to match system vendor if not set
    if (!dmi->chassis.vendor[0] && dmi->sys.vendor[0])
        safecpy(dmi->chassis.vendor, dmi->sys.vendor);

    // Set bios vendor to match system vendor if not set
    if (!dmi->bios.vendor[0] && dmi->sys.vendor[0])
        safecpy(dmi->bios.vendor, dmi->sys.vendor);

    // Default product name if all methods failed
    if (!dmi->product.name[0])
        safecpy(dmi->product.name, "Mac");

    // Default chassis type if we couldn't determine it
    if (!dmi->chassis.type[0])
        safecpy(dmi->chassis.type, "3"); // Desktop
        
    // Check boot mode (UEFI vs Legacy) on macOS
    io_registry_entry_t options = IORegistryEntryFromPath(kIOMasterPortDefault, "IODeviceTree:/options");
    if (options) {
        bool found_efi = false;
        CFTypeRef property = IORegistryEntryCreateCFProperty(options, CFSTR("efi-boot-device"), kCFAllocatorDefault, 0);
        if (property) {
            found_efi = true;
            CFRelease(property);
        }
        
        if (found_efi) {
            safecpy(dmi->bios.mode, "UEFI");
            
            // Check secure boot status
            CFTypeRef secure_boot_prop = IORegistryEntryCreateCFProperty(options, 
                                                                    CFSTR("SecureBootLevel"), 
                                                                    kCFAllocatorDefault, 0);
            if (secure_boot_prop) {
                if (CFGetTypeID(secure_boot_prop) == CFNumberGetTypeID()) {
                    int level;
                    if (CFNumberGetValue((CFNumberRef)secure_boot_prop, kCFNumberIntType, &level)) {
                        // Any positive value indicates secure boot is enabled
                        dmi->bios.secure_boot = (level > 0);
                    }
                }
                CFRelease(secure_boot_prop);
            }
        } else {
            safecpy(dmi->bios.mode, "Legacy");
        }
        
        IOObjectRelease(options);
    } else {
        safecpy(dmi->bios.mode, "Unknown");
    }
    
    // Check for Apple T2 security chip (similar to TPM but without using external commands)
    // This uses only IOKit APIs which are safe and built-in
    io_registry_entry_t chip = IORegistryEntryFromPath(kIOMasterPortDefault, "IOService:/AppleACPIPlatformExpert/SMC/AppleT2:");
    if (chip) {
        // If T2 chip is present, secure boot is likely enabled
        dmi->bios.secure_boot = true;
        IOObjectRelease(chip);
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

void os_dmi_info_get(DMI_INFO *dmi) {
    if (!dmi) return;
    
    // System information from SMBIOS
    freebsd_get_sysctl_str("hw.vendor", dmi->sys.vendor, sizeof(dmi->sys.vendor));
    freebsd_get_sysctl_str("hw.product", dmi->product.name, sizeof(dmi->product.name));
    freebsd_get_sysctl_str("hw.version", dmi->product.version, sizeof(dmi->product.version));
    freebsd_get_sysctl_str("hw.serial", dmi->sys.serial, sizeof(dmi->sys.serial));

    // Try using kenv for additional information
    freebsd_get_kenv_str("smbios.system.maker", dmi->sys.vendor, sizeof(dmi->sys.vendor));
    freebsd_get_kenv_str("smbios.system.product", dmi->product.name, sizeof(dmi->product.name));
    freebsd_get_kenv_str("smbios.system.version", dmi->product.version, sizeof(dmi->product.version));
    freebsd_get_kenv_str("smbios.system.sku", dmi->product.sku, sizeof(dmi->product.sku));
    freebsd_get_kenv_str("smbios.system.family", dmi->product.family, sizeof(dmi->product.family));
    freebsd_get_kenv_str("smbios.system.serial", dmi->sys.serial, sizeof(dmi->sys.serial));

    // Board information
    freebsd_get_kenv_str("smbios.planar.maker", dmi->board.vendor, sizeof(dmi->board.vendor));
    freebsd_get_kenv_str("smbios.planar.product", dmi->board.name, sizeof(dmi->board.name));
    freebsd_get_kenv_str("smbios.planar.version", dmi->board.version, sizeof(dmi->board.version));
    freebsd_get_kenv_str("smbios.planar.serial", dmi->board.serial, sizeof(dmi->board.serial));

    // BIOS information
    freebsd_get_kenv_str("smbios.bios.vendor", dmi->bios.vendor, sizeof(dmi->bios.vendor));
    freebsd_get_kenv_str("smbios.bios.version", dmi->bios.version, sizeof(dmi->bios.version));
    freebsd_get_kenv_str("smbios.bios.reldate", dmi->bios.date, sizeof(dmi->bios.date));
    freebsd_get_kenv_str("smbios.bios.release", dmi->bios.release, sizeof(dmi->bios.release));

    // Chassis information
    freebsd_get_kenv_str("smbios.chassis.maker", dmi->chassis.vendor, sizeof(dmi->chassis.vendor));
    freebsd_get_kenv_str("smbios.chassis.version", dmi->chassis.version, sizeof(dmi->chassis.version));
    freebsd_get_kenv_str("smbios.chassis.serial", dmi->chassis.serial, sizeof(dmi->chassis.serial));

    // Chassis type
    freebsd_get_kenv_str("smbios.chassis.type", dmi->chassis.type, sizeof(dmi->chassis.type));

    // If we couldn't get system information from SMBIOS, try to use model
    if (!dmi->product.name[0])
        freebsd_get_sysctl_str("hw.model", dmi->product.name, sizeof(dmi->product.name));
    
    // Try to get asset tags
    freebsd_get_kenv_str("smbios.system.asset_tag", dmi->sys.asset_tag, sizeof(dmi->sys.asset_tag));
    freebsd_get_kenv_str("smbios.chassis.asset_tag", dmi->chassis.asset_tag, sizeof(dmi->chassis.asset_tag));
    freebsd_get_kenv_str("smbios.planar.asset_tag", dmi->board.asset_tag, sizeof(dmi->board.asset_tag));
    
    // Get UUID
    freebsd_get_kenv_str("smbios.system.uuid", dmi->sys.uuid, sizeof(dmi->sys.uuid));
    
    // Check for UEFI boot mode using sysctl (no external commands)
    char bootmethod[64] = {0};
    size_t bootmethod_size = sizeof(bootmethod) - 1;
    if (sysctlbyname("kern.bootmethod", bootmethod, &bootmethod_size, NULL, 0) == 0) {
        if (strncmp(bootmethod, "UEFI", 4) == 0) {
            safecpy(dmi->bios.mode, "UEFI");
            
            // Check secure boot status - FreeBSD stores this in sysctl (no external commands)
            int secure_boot_enabled = 0;
            size_t secboot_size = sizeof(secure_boot_enabled);
            if (sysctlbyname("kern.secureboot.enable", &secure_boot_enabled, &secboot_size, NULL, 0) == 0) {
                dmi->bios.secure_boot = (secure_boot_enabled != 0);
            }
        } else {
            safecpy(dmi->bios.mode, "Legacy");
        }
    } else {
        // Fallback: check for EFI presence (older FreeBSD versions) - no external commands
        int efi_present = 0;
        size_t efi_size = sizeof(efi_present);
        if (sysctlbyname("kern.efi.runtime", &efi_present, &efi_size, NULL, 0) == 0) {
            if (efi_present) {
                safecpy(dmi->bios.mode, "UEFI");
            } else {
                safecpy(dmi->bios.mode, "Legacy");
            }
        }
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
                                  DMI_INFO *dmi,
                                  const char *string_table,
                                  const BYTE *smbios_data,
                                  DWORD smbios_size) {
    if (header->length < 18 || !dmi) // Minimum size for BIOS info
        return;
        
    const BYTE *data = (const BYTE *)header;
    char temp_str[256];
    
    // BIOS Vendor (string index at offset 4)
    if (data[4] > 0 && get_smbios_string(smbios_data, smbios_size, string_table, 
                                     data[4], temp_str, sizeof(temp_str))) {
        safecpy(dmi->bios.vendor, temp_str);
    }
    
    // BIOS Version (string index at offset 5)
    if (data[5] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[5], temp_str, sizeof(temp_str))) {
        safecpy(dmi->bios.version, temp_str);
    }
    
    // BIOS Release Date (string index at offset 8)
    if (data[8] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[8], temp_str, sizeof(temp_str))) {
        safecpy(dmi->bios.date, temp_str);
    }
}

// Process System Information (Type 1)
static void process_smbios_system_info(const smbios_header_t *header,
                                    DMI_INFO *dmi,
                                    const char *string_table,
                                    const BYTE *smbios_data,
                                    DWORD smbios_size) {
    if (header->length < 8 || !dmi) // Minimum size for System info
        return;
        
    const BYTE *data = (const BYTE *)header;
    char temp_str[256];
    
    // Manufacturer (string index at offset 4)
    if (data[4] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[4], temp_str, sizeof(temp_str))) {
        safecpy(dmi->sys.vendor, temp_str);
    }
    
    // Product Name (string index at offset 5)
    if (data[5] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[5], temp_str, sizeof(temp_str))) {
        safecpy(dmi->product.name, temp_str);
    }
    
    // Version (string index at offset 6)
    if (data[6] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[6], temp_str, sizeof(temp_str))) {
        safecpy(dmi->product.version, temp_str);
    }
    
    // Serial Number (string index at offset 7)
    if (data[7] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[7], temp_str, sizeof(temp_str))) {
        safecpy(dmi->sys.serial, temp_str);
    }
    
    // If structure is long enough for family (SMBIOS 2.1+)
    if (header->length >= 25 && data[21] > 0 && 
        get_smbios_string(smbios_data, smbios_size, string_table,
                       data[21], temp_str, sizeof(temp_str))) {
        safecpy(dmi->product.family, temp_str);
    }
    
    // Extract system UUID (16 bytes starting at offset 8)
    if (header->length >= 24) {
        // Verify we have enough data to access all bytes safely
        if ((uintptr_t)data + 23 < (uintptr_t)smbios_data + smbios_size) {
            // UUID is stored as 16 bytes, we'll format it as a standard UUID string
            char uuid_str[40] = {0}; // 36 chars for UUID + null terminator
            
            // Note: SMBIOS UUID needs byte swapping for first three fields
            int ret = snprintf(uuid_str, sizeof(uuid_str),
                     "%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
                     data[11], data[10], data[9], data[8],         // field 1 (byte swapped)
                     data[13], data[12],                           // field 2 (byte swapped)
                     data[15], data[14],                           // field 3 (byte swapped)
                     data[16], data[17],                           // field 4 (not swapped)
                     data[18], data[19], data[20], data[21], data[22], data[23]); // field 5 (not swapped)
            
            // Verify the formatting worked and buffer wasn't truncated
            if (ret > 0 && ret < (int)sizeof(uuid_str)) {
                // Only set if not all zeros (some systems report all zeros)
                if (uuid_str[0] != '0' || uuid_str[1] != '0' || uuid_str[2] != '0' || uuid_str[3] != '0') {
                    safecpy(dmi->sys.uuid, uuid_str);
                }
            }
        }
    }
    
    // Asset Tag is sometimes available in newer SMBIOS versions
    if (header->length >= 27 && data[26] > 0 &&
        get_smbios_string(smbios_data, smbios_size, string_table,
                       data[26], temp_str, sizeof(temp_str))) {
        safecpy(dmi->sys.asset_tag, temp_str);
    }
}

// Process Baseboard Information (Type 2)
static void process_smbios_baseboard_info(const smbios_header_t *header,
                                       DMI_INFO *dmi,
                                       const char *string_table,
                                       const BYTE *smbios_data,
                                       DWORD smbios_size) {
    if (header->length < 8 || !dmi) // Minimum size for Baseboard info
        return;
        
    const BYTE *data = (const BYTE *)header;
    char temp_str[256];
    
    // Manufacturer (string index at offset 4)
    if (data[4] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[4], temp_str, sizeof(temp_str))) {
        safecpy(dmi->board.vendor, temp_str);
    }
    
    // Product (string index at offset 5)
    if (data[5] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[5], temp_str, sizeof(temp_str))) {
        safecpy(dmi->board.name, temp_str);
    }
    
    // Version (string index at offset 6)
    if (data[6] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[6], temp_str, sizeof(temp_str))) {
        safecpy(dmi->board.version, temp_str);
    }
    
    // Serial Number (string index at offset 7)
    if (data[7] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[7], temp_str, sizeof(temp_str))) {
        safecpy(dmi->board.serial, temp_str);
    }
    
    // Asset Tag (string index at offset 8)
    if (header->length >= 9 && data[8] > 0 &&
        get_smbios_string(smbios_data, smbios_size, string_table,
                      data[8], temp_str, sizeof(temp_str))) {
        safecpy(dmi->board.asset_tag, temp_str);
    }
}

// Process Chassis Information (Type 3)
static void process_smbios_chassis_info(const smbios_header_t *header,
                                     DMI_INFO *dmi,
                                     const char *string_table,
                                     const BYTE *smbios_data,
                                     DWORD smbios_size) {
    if (header->length < 9 || !dmi) // Minimum size for Chassis info
        return;
        
    const BYTE *data = (const BYTE *)header;
    char temp_str[256];
    
    // Manufacturer (string index at offset 4)
    if (data[4] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[4], temp_str, sizeof(temp_str))) {
        safecpy(dmi->chassis.vendor, temp_str);
    }
    
    // Type (numerical value at offset 5)
    BYTE chassis_type = data[5] & 0x7F; // Mask out MSB
    snprintf(dmi->chassis.type, sizeof(dmi->chassis.type), "%d", chassis_type);
    
    // Version (string index at offset 6)
    if (data[6] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[6], temp_str, sizeof(temp_str))) {
        safecpy(dmi->chassis.version, temp_str);
    }
    
    // Serial Number (string index at offset 7)
    if (data[7] > 0 && get_smbios_string(smbios_data, smbios_size, string_table,
                                     data[7], temp_str, sizeof(temp_str))) {
        safecpy(dmi->chassis.serial, temp_str);
    }
    
    // Asset Tag (string index at offset 8)
    if (header->length >= 9 && data[8] > 0 &&
        get_smbios_string(smbios_data, smbios_size, string_table, 
                      data[8], temp_str, sizeof(temp_str))) {
        safecpy(dmi->chassis.asset_tag, temp_str);
    }
}

// Process a complete SMBIOS structure
static void process_smbios_structure(const smbios_header_t *header, 
                                  DMI_INFO *dmi,
                                  const char *string_table,
                                  const BYTE *smbios_data,
                                  DWORD smbios_size) {
    if (!dmi) return;
    
    switch (header->type) {
        case 0: // BIOS Information
            process_smbios_bios_info(header, dmi, string_table, smbios_data, smbios_size);
            break;
            
        case 1: // System Information
            process_smbios_system_info(header, dmi, string_table, smbios_data, smbios_size);
            break;
            
        case 2: // Baseboard Information
            process_smbios_baseboard_info(header, dmi, string_table, smbios_data, smbios_size);
            break;
            
        case 3: // Chassis Information
            process_smbios_chassis_info(header, dmi, string_table, smbios_data, smbios_size);
            break;
    }
}

// Parse all SMBIOS structures with robust error handling
static void parse_smbios_structures(const smbios_data_t smbios, DMI_INFO *dmi) {
    if (!smbios.valid || !smbios.data || smbios.size < 8 || !dmi)
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
            process_smbios_structure(header, dmi, string_table, smbios.data, smbios.size);
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
static void windows_get_smbios_info(DMI_INFO *dmi) {
    if (!dmi) return;
    
    // Get SMBIOS data using our improved container structure
    smbios_data_t smbios = get_smbios_data();
    if (!smbios.valid)
        return;
        
    // Process the SMBIOS data with our improved parser
    parse_smbios_structures(smbios, dmi);
    
    // Clean up
    freez(smbios.data);
}

// Fallback method using registry
static void windows_get_registry_info(DMI_INFO *dmi) {
    if (!dmi) return;
    
    // System manufacturer and model from registry
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "SystemManufacturer",
        dmi->sys.vendor,
        sizeof(dmi->sys.vendor)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "SystemProductName",
        dmi->product.name,
        sizeof(dmi->product.name)
    );
    
    // System Serial Number
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "SystemSerialNumber",
        dmi->sys.serial,
        sizeof(dmi->sys.serial)
    );

    // BIOS information
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BIOSVendor",
        dmi->bios.vendor,
        sizeof(dmi->bios.vendor)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BIOSVersion",
        dmi->bios.version,
        sizeof(dmi->bios.version)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BIOSReleaseDate",
        dmi->bios.date,
        sizeof(dmi->bios.date)
    );

    // Board information
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BaseBoardManufacturer",
        dmi->board.vendor,
        sizeof(dmi->board.vendor)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BaseBoardProduct",
        dmi->board.name,
        sizeof(dmi->board.name)
    );

    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BaseBoardVersion",
        dmi->board.version,
        sizeof(dmi->board.version)
    );
    
    // Base Board Serial
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "BaseBoardSerialNumber",
        dmi->board.serial,
        sizeof(dmi->board.serial)
    );
    
    // Chassis Serial (might not be available in registry, but try anyway)
    windows_read_registry_string(
        HKEY_LOCAL_MACHINE,
        "HARDWARE\\DESCRIPTION\\System\\BIOS",
        "ChassisSerialNumber",
        dmi->chassis.serial,
        sizeof(dmi->chassis.serial)
    );
    
    // Get BIOS boot mode (UEFI or Legacy)
    DWORD firmware_type = 0;
    if (GetFirmwareType(&firmware_type)) {
        switch (firmware_type) {
            case 1: // FirmwareTypeBios
                safecpy(dmi->bios.mode, "Legacy");
                break;
            case 2: // FirmwareTypeUefi
                safecpy(dmi->bios.mode, "UEFI");
                break;
            default:
                safecpy(dmi->bios.mode, "Unknown");
        }
    }
    
    // Check if secure boot is enabled
    HKEY key;
    if (RegOpenKeyExA(HKEY_LOCAL_MACHINE, "SYSTEM\\CurrentControlSet\\Control\\SecureBoot\\State", 
                     0, KEY_READ, &key) == ERROR_SUCCESS) {
        DWORD secure_boot_value = 0;
        DWORD size = sizeof(secure_boot_value);
        if (RegQueryValueExA(key, "UEFISecureBootEnabled", NULL, NULL, 
                            (LPBYTE)&secure_boot_value, &size) == ERROR_SUCCESS) {
            dmi->bios.secure_boot = (secure_boot_value != 0);
        }
        RegCloseKey(key);
    }
}

// Main function to get hardware information
void os_dmi_info_get(DMI_INFO *dmi) {
    if (!dmi) return;
    
    // First try SMBIOS data through firmware table API
    windows_get_smbios_info(dmi);

    // Try registry as a fallback or to fill missing values
    windows_get_registry_info(dmi);

    // Get asset tags if not set by SMBIOS
    if (!dmi->sys.asset_tag[0]) {
        windows_read_registry_string(
            HKEY_LOCAL_MACHINE,
            "HARDWARE\\DESCRIPTION\\System\\BIOS",
            "SystemAssetTag",
            dmi->sys.asset_tag,
            sizeof(dmi->sys.asset_tag)
        );
    }
    
    if (!dmi->board.asset_tag[0]) {
        windows_read_registry_string(
            HKEY_LOCAL_MACHINE,
            "HARDWARE\\DESCRIPTION\\System\\BIOS",
            "BaseBoardAssetTag",
            dmi->board.asset_tag,
            sizeof(dmi->board.asset_tag)
        );
    }
    
    if (!dmi->chassis.asset_tag[0]) {
        windows_read_registry_string(
            HKEY_LOCAL_MACHINE,
            "HARDWARE\\DESCRIPTION\\System\\BIOS",
            "ChassisAssetTag",
            dmi->chassis.asset_tag,
            sizeof(dmi->chassis.asset_tag)
        );
    }
    
    // Check for Secure Boot - Read directly from registry, no external commands
    HKEY secureBootKey;
    if (RegOpenKeyExA(HKEY_LOCAL_MACHINE, "SYSTEM\\CurrentControlSet\\Control\\SecureBoot\\State", 
                     0, KEY_READ, &secureBootKey) == ERROR_SUCCESS) {
        DWORD secure_boot = 0;
        DWORD size = sizeof(secure_boot);
        if (RegQueryValueExA(secureBootKey, "UEFISecureBootEnabled", NULL, NULL, 
                            (LPBYTE)&secure_boot, &size) == ERROR_SUCCESS) {
            dmi->bios.secure_boot = (secure_boot != 0);
        }
        RegCloseKey(secureBootKey);
    }

    // If chassis type is not set or not a valid number, set a default
    if (!dmi->chassis.type[0] || atoi(dmi->chassis.type) <= 0) {
        // Check common system names for laptops
        if (strcasestr(dmi->product.name, "notebook") != NULL ||
            strcasestr(dmi->product.name, "laptop") != NULL ||
            strcasestr(dmi->product.name, "book") != NULL) {
            safecpy(dmi->chassis.type, "9");  // Laptop
        }
        // Check for servers
        else if (strcasestr(dmi->product.name, "server") != NULL) {
            safecpy(dmi->chassis.type, "17");  // Server
        }
        // Default to desktop
        else {
            safecpy(dmi->chassis.type, "3");  // Desktop
        }
    }
}

#else
void os_dmi_info_get(DMI_INFO *dmi) {
    // No implementation for this platform
}
#endif

// --------------------------------------------------------------------------------------------------------------------
// public API

void dmi_info_init(DMI_INFO *dmi) {
    if (!dmi) return;
    
    // System information
    dmi->sys.vendor[0] = '\0';
    dmi->sys.serial[0] = '\0';
    dmi->sys.uuid[0] = '\0';
    dmi->sys.asset_tag[0] = '\0';
    
    // Product information
    dmi->product.name[0] = '\0';
    dmi->product.version[0] = '\0';
    dmi->product.sku[0] = '\0';
    dmi->product.family[0] = '\0';
    
    // Board information
    dmi->board.vendor[0] = '\0';
    dmi->board.name[0] = '\0';
    dmi->board.version[0] = '\0';
    dmi->board.serial[0] = '\0';
    dmi->board.asset_tag[0] = '\0';
    
    // BIOS information
    dmi->bios.vendor[0] = '\0';
    dmi->bios.version[0] = '\0';
    dmi->bios.date[0] = '\0';
    dmi->bios.release[0] = '\0';
    dmi->bios.mode[0] = '\0';
    dmi->bios.secure_boot = false;
    
    // Chassis information
    dmi->chassis.vendor[0] = '\0';
    dmi->chassis.version[0] = '\0';
    dmi->chassis.type[0] = '\0';
    dmi->chassis.serial[0] = '\0';
    dmi->chassis.asset_tag[0] = '\0';
}
