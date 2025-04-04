// SPDX-License-Identifier: GPL-3.0-or-later

#include "status-file-dmi.h"
#include "status-file-product.h"

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
    else if(dmi_is_virtual_machine(&ds->hw))
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
