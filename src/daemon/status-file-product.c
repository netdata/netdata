// SPDX-License-Identifier: GPL-3.0-or-later

#include "status-file-dmi.h"
#include "status-file-product.h"

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
        {"Dell EMC", "Dell"},

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

        {"IceWhale Technology Co.,Ltd.", "IceWhale"},

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

static bool dmi_is_virtual_machine(const DMI_INFO *dmi) {
    if(!dmi) return false;

    const char *vm_indicators[] = {
        "Virt", "KVM", "vServer", "Cloud", "Hyper", "Droplet",
        "Compute ", // with a space to not match "Computer"
        "HVM domU", "Parallels", "(i440FX", "(q35", "OpenStack", "QEMU",
        "VMWare", "DigitalOcean", "Oracle", "Linode", "Amazon EC2"
    };

    const char *strs_to_check[] = {
        dmi->product.name,
        dmi->product.family,
        dmi->sys.vendor,
        dmi->board.name,
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

// Check for active ECC memory controllers
static bool has_ecc_memory(void) {
#if defined(OS_LINUX)
    char edac_path[FILENAME_MAX + 1];
    snprintfz(edac_path, FILENAME_MAX, "%s/sys/devices/system/edac/mc", netdata_configured_host_prefix ? netdata_configured_host_prefix : "");

    DIR *dir = opendir(edac_path);
    if (!dir)
        return false;

    struct dirent *entry;
    bool found_mc = false;

    while ((entry = readdir(dir))) {
        // Look for "mc0", "mc1", etc. directories
        if (strncmp(entry->d_name, "mc", 2) == 0 && isdigit(entry->d_name[2])) {
            // Check for presence of ECC-related files in this mc directory
            char mc_path[FILENAME_MAX + 1];
            snprintfz(mc_path, FILENAME_MAX, "%s/%s/ce_count", edac_path, entry->d_name);

            struct stat st;
            if (stat(mc_path, &st) == 0) {
                found_mc = true;
                break;
            }
        }
    }

    closedir(dir);
    return found_mc;
#else
    return false; // Not implemented for this OS
#endif
}

// Check for IPMI device
static bool has_ipmi(void) {
#if defined(OS_LINUX)
    char ipmi_dev_path[FILENAME_MAX + 1];
    snprintfz(ipmi_dev_path, FILENAME_MAX, "%s/dev/ipmi0", netdata_configured_host_prefix ? netdata_configured_host_prefix : "");
    struct stat ipmi_stat;
    return (stat(ipmi_dev_path, &ipmi_stat) == 0);
#else
    return false; // Not implemented for this OS
#endif
}

// Check for multiple CPU sockets
static bool has_multiple_cpu_sockets(void) {
#if defined(OS_LINUX)
    char cpu_path[FILENAME_MAX + 1];
    snprintfz(cpu_path, FILENAME_MAX, "%s/sys/devices/system/cpu", netdata_configured_host_prefix ? netdata_configured_host_prefix : "");

    DIR *dir = opendir(cpu_path);
    if (!dir)
        return false;

    DICTIONARY *physical_ids = dictionary_create(DICT_OPTION_SINGLE_THREADED);
    struct dirent *entry;

    while ((entry = readdir(dir))) {
        // Check for cpu directories (cpu0, cpu1, etc.)
        if (strncmp(entry->d_name, "cpu", 3) == 0 && isdigit(entry->d_name[3])) {
            char topology_path[FILENAME_MAX + 1];
            char physical_id[64];

            // Read the physical_package_id file for this CPU
            snprintfz(topology_path, FILENAME_MAX, "%s/%s/topology/physical_package_id",
                      cpu_path, entry->d_name);

            if (read_txt_file(topology_path, physical_id, sizeof(physical_id)) == 0) {
                dictionary_set(physical_ids, physical_id, NULL, 0);
            }
        }
    }

    closedir(dir);

    unsigned int socket_count = dictionary_entries(physical_ids);
    dictionary_destroy(physical_ids);

    return (socket_count > 1);
#else
    return false; // Not implemented for this OS
#endif
}

// Main function to check for server hardware indicators
static bool is_server_hardware(void) {
    // Check each server indicator
    if (has_ecc_memory())
        return true;

    if (has_ipmi())
        return true;

    if (has_multiple_cpu_sockets())
        return true;

    return false; // No server indicators found
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
                force_type = "mini-pc";
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
        // copy the product family
        size_t len = strcatz(ds->product.name, 0, ds->hw.product.family, sizeof(ds->product.name));

        // append the product name, if it is not included already
        if(ds->hw.product.name[0] && !strcasestr(ds->product.name, ds->hw.product.name)) {
            if(ds->product.name[0]) {
                bool includes_family = strcasestr(ds->hw.product.name, ds->hw.product.family) != NULL;

                if(includes_family) {
                    // the product name includes the product family we have in buf
                    // we can clear the family and add only the product name
                    len = 0;
                }
                else {
                    // the product name is not included in the product family
                    // we append a separator, and later the name
                    len = strcatz(ds->product.name, len, " / ", sizeof(ds->product.name));
                }
            }

            len = strcatz(ds->product.name, len, ds->hw.product.name, sizeof(ds->product.name));
        }

        // append the board name, if it is not included already
        if(ds->hw.board.name[0] && !strcasestr(ds->product.name, ds->hw.board.name)) {
            if(ds->product.name[0]) {
                bool includes_family = !ds->hw.product.family[0] || strcasestr(ds->hw.board.name, ds->hw.product.family) != NULL;
                bool includes_product = !ds->hw.product.name[0] || strcasestr(ds->hw.board.name, ds->hw.product.name) != NULL;

                if(includes_family && includes_product) {
                    // the board name includes both the product and the family
                    // we can clear buf and add only the board name
                    len = 0;
                }
                else {
                    // the board name is not included in buf
                    // we append a separator, and later the name
                    len = strcatz(ds->product.name, len, " / ", sizeof(ds->product.name));
                }
            }

            len = strcatz(ds->product.name, len, ds->hw.board.name, sizeof(ds->product.name));
        }

        if(!ds->product.name[0])
            safecpy(ds->product.name, "unknown");
    }

    if(ds->virtualization[0] && strcasecmp(ds->virtualization, "none") != 0 && strcasecmp(ds->virtualization, "unknown") != 0)
        safecpy(ds->product.type, "vm");
    else if(force_type)
        safecpy(ds->product.type, force_type);
    else if(dmi_is_virtual_machine(&ds->hw))
        safecpy(ds->product.type, "vm");
    else if(is_server_hardware())
        safecpy(ds->product.type, "server");
    else {
        char *end = NULL;
        int type = (int)strtol(ds->hw.chassis.type, &end, 10);
        if(type && (!end || !*end))
            safecpy(ds->product.type, dmi_chassis_type_to_string(type));
        else
            safecpy(ds->product.type, "unknown");
    }
}
