// SPDX-License-Identifier: GPL-3.0-or-later

#include "status-file-dmi.h"

static void dmi_info(const char *file, const char *alt, char *dst, size_t dst_size) {
    char filename[FILENAME_MAX];
    dst[0] = '\0';

    if (netdata_configured_host_prefix && *netdata_configured_host_prefix) {
        snprintfz(filename, sizeof(filename), "%s/sys/class/dmi/id/%s", netdata_configured_host_prefix, file);
        if (access(filename, R_OK) != 0) {
            snprintfz(
                filename, sizeof(filename), "%s/sys/devices/virtual/dmi/id/%s", netdata_configured_host_prefix, file);
            if (access(filename, R_OK) != 0)
                filename[0] = '\0';
        }
    } else
        filename[0] = '\0';

    if (!filename[0]) {
        snprintfz(filename, sizeof(filename), "/sys/class/dmi/id/%s", file);
        if (access(filename, R_OK) != 0) {
            snprintfz(filename, sizeof(filename), "/sys/devices/virtual/dmi/id/%s", file);
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

    struct {
        const char *found;
        const char *replace;
    } replacements[] = {
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

    for (size_t i = 0; i < _countof(replacements); i++) {
        if (strcasecmp(buf, replacements[i].found) == 0) {
            strncpyz(buf, replacements[i].replace, sizeof(buf) - 1);
            break;
        }
    }

    if (!buf[0])
        return;

    if (strstr(file, "_vendor") != NULL) {
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
                strncpyz(buf, vendors[i].replace, sizeof(buf) - 1);
                break;
            }
        }
    }

    // copy it to its final location
    strncpyz(dst, buf, dst_size - 1);
}

static void fill_dmi_chassis_type(DAEMON_STATUS_FILE *ds) {
    dmi_info("chassis_type", NULL, ds->hw.chassis.type, sizeof(ds->hw.chassis.type));
    int chassis_type = atoi(ds->hw.chassis.type);

    const char *str = NULL;

    // we detect here VMs, but we also overwrite this from system-info.sh later
    if(strcasestr(ds->hw.product.family, "Virt") != NULL ||
        strcasestr(ds->hw.product.name, "KVM") != NULL ||
        strcasestr(ds->hw.product.name, "Virt") != NULL ||
        strcasestr(ds->hw.product.name, "vServer") != NULL ||
        strcasestr(ds->hw.product.name, "Cloud") != NULL ||
        strcasestr(ds->hw.product.name, "Hyper") != NULL ||
        strcasestr(ds->hw.product.name, "Droplet") != NULL ||
        strcasestr(ds->hw.product.name, "Compute") != NULL ||
        strcasestr(ds->hw.product.name, "(i440FX") != NULL ||
        strcasestr(ds->hw.product.name, "(q35") != NULL ||
        strcasestr(ds->hw.product.name, "OpenStack") != NULL ||
        strcasestr(ds->hw.board.name, "Virtual Machine") != NULL ||
        strcasestr(ds->hw.sys.vendor, "QEMU") != NULL ||
        strcasestr(ds->hw.sys.vendor, "kvm") != NULL ||
        strcasestr(ds->hw.sys.vendor, "VMWare") != NULL ||
        strcasestr(ds->hw.sys.vendor, "DigitalOcean") != NULL ||
        strcasestr(ds->hw.sys.vendor, "Oracle") != NULL ||
        strcasestr(ds->hw.sys.vendor, "Linode") != NULL ||
        strcasestr(ds->hw.sys.vendor, "Amazon EC2") != NULL) {
        str = "vm";
    }

    if(!str) {
        // original info from SMBIOS
        // https://www.dmtf.org/sites/default/files/standards/documents/DSP0134_3.2.0.pdf
        // selected values aligned with inxi: https://github.com/smxi/inxi/blob/master/inxi
        switch(chassis_type) {
            case 1: str = "other"; break;
            case 2: str = "unknown"; break;
            case 3: str = "desktop"; break;
            case 4: str = "desktop" /* "low-profile-desktop" */; break;
            case 5: str = "pizza-box"; break; // was a 1 U desktop enclosure, but some old laptops also id this way
            case 6: str = "desktop" /* "mini-tower-desktop" */; break;
            case 7: str = "desktop" /* "tower-desktop" */; break;
            case 8: str = "portable"; break;
            case 9: str = "laptop"; break;
            case 10: str = "laptop" /* "notebook" */; break;
            case 11: str = "portable" /* "hand-held" */; break;
            case 12: str = "docking-station"; break;
            case 13: str = "desktop" /* "all-in-one" */; break;
            case 14: str = "notebook" /* "sub-notebook" */; break;
            case 15: str = "desktop" /* "space-saving-desktop" */; break;
            case 16: str = "laptop" /* "lunch-box" */; break;
            case 17: str = "server" /* "main-server-chassis" */; break;
            case 18: str = "expansion-chassis"; break;
            case 19: str = "sub-chassis"; break;
            case 20: str = "bus-expansion"; break;
            case 21: str = "peripheral"; break;
            case 22: str = "raid"; break;
            case 23: str = "server" /* "rack-mount-server" */; break;
            case 24: str = "desktop" /* "sealed-desktop" */; break;
            case 25: str = "multimount-chassis"; break;
            case 26: str = "compact-pci"; break;
            case 27: str = "blade" /* "advanced-tca" */; break;
            case 28: str = "blade"; break;
            case 29: str = "blade-enclosure"; break;
            case 30: str = "tablet"; break;
            case 31: str = "convertible"; break;
            case 32: str = "detachable"; break;
            case 33: str = "iot-gateway"; break;
            case 34: str = "embedded-pc"; break;
            case 35: str = "mini-pc"; break;
            case 36: str = "stick-pc"; break;
            default: str = NULL; break; // let it be numeric
        }
    }

    if(str)
        strncpyz(ds->hw.chassis.type, str, sizeof(ds->hw.chassis.type) - 1);
}

void fill_dmi_info(DAEMON_STATUS_FILE *ds) {
    dmi_info("sys_vendor", NULL, ds->hw.sys.vendor, sizeof(ds->hw.sys.vendor));

    dmi_info("product_name", "/proc/device-tree/model", ds->hw.product.name, sizeof(ds->hw.product.name));
    dmi_info("product_version", NULL, ds->hw.product.version, sizeof(ds->hw.product.version));
    dmi_info("product_sku", NULL, ds->hw.product.sku, sizeof(ds->hw.product.sku));
    dmi_info("product_family", NULL, ds->hw.product.family, sizeof(ds->hw.product.family));

    dmi_info("chassis_vendor", NULL, ds->hw.chassis.vendor, sizeof(ds->hw.chassis.vendor));
    dmi_info("chassis_version", NULL, ds->hw.chassis.version, sizeof(ds->hw.chassis.version));

    dmi_info("board_vendor", NULL, ds->hw.board.vendor, sizeof(ds->hw.board.vendor));
    dmi_info("board_name", NULL, ds->hw.board.name, sizeof(ds->hw.board.name));
    dmi_info("board_version", NULL, ds->hw.board.version, sizeof(ds->hw.board.version));

    dmi_info("bios_vendor", NULL, ds->hw.bios.vendor, sizeof(ds->hw.bios.vendor));
    dmi_info("bios_version", NULL, ds->hw.bios.version, sizeof(ds->hw.bios.version));
    dmi_info("bios_date", NULL, ds->hw.bios.date, sizeof(ds->hw.bios.date));
    dmi_info("bios_release", NULL, ds->hw.bios.release, sizeof(ds->hw.bios.release));

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

    fill_dmi_chassis_type(ds);
}
