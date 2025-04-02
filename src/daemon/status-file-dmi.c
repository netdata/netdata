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
void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    ;
}
#elif defined(OS_FREEBSD)
void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    ;
}
#elif defined(OS_WINDOWS)
void os_dmi_info(DAEMON_STATUS_FILE *ds) {
    ;
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
