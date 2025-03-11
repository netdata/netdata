// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_proc.h"

#define PLUGIN_PROC_MODULE_DRM_NAME "/sys/class/drm"
#define CONFIG_SECTION_PLUGIN_PROC_DRM "plugin:proc:/sys/class/drm"
#define AMDGPU_CHART_TYPE "amdgpu"

struct amdgpu_id_struct {
    unsigned long long asic_id;
    unsigned long long pci_rev_id;
    const char *marketing_name;
};

/* 
 * About amdgpu_ids list:
 * ------------------------------------------------------------------------
 * Copyright (C) 2023 Advanced Micro Devices, Inc. All rights reserved.
 *
 * The list is copied from:
 * https://raw.githubusercontent.com/Syllo/nvtop/master/src/amdgpu_ids.h
 * 
 * which is modified from libdrm (MIT License):
 *
 * URL: https://gitlab.freedesktop.org/mesa/drm/-/blob/main/data/amdgpu.ids
 * ------------------------------------------------------------------------
 * **IMPORTANT**: The amdgpu_ids has to be modified after new GPU releases. 
 * ------------------------------------------------------------------------*/

static const struct amdgpu_id_struct amdgpu_ids[] = {
    {0x1309, 0x00, "AMD Radeon R7 Graphics"},
    {0x130A, 0x00, "AMD Radeon R6 Graphics"},
    {0x130B, 0x00, "AMD Radeon R4 Graphics"},
    {0x130C, 0x00, "AMD Radeon R7 Graphics"},
    {0x130D, 0x00, "AMD Radeon R6 Graphics"},
    {0x130E, 0x00, "AMD Radeon R5 Graphics"},
    {0x130F, 0x00, "AMD Radeon R7 Graphics"},
    {0x130F, 0xD4, "AMD Radeon R7 Graphics"},
    {0x130F, 0xD5, "AMD Radeon R7 Graphics"},
    {0x130F, 0xD6, "AMD Radeon R7 Graphics"},
    {0x130F, 0xD7, "AMD Radeon R7 Graphics"},
    {0x1313, 0x00, "AMD Radeon R7 Graphics"},
    {0x1313, 0xD4, "AMD Radeon R7 Graphics"},
    {0x1313, 0xD5, "AMD Radeon R7 Graphics"},
    {0x1313, 0xD6, "AMD Radeon R7 Graphics"},
    {0x1315, 0x00, "AMD Radeon R5 Graphics"},
    {0x1315, 0xD4, "AMD Radeon R5 Graphics"},
    {0x1315, 0xD5, "AMD Radeon R5 Graphics"},
    {0x1315, 0xD6, "AMD Radeon R5 Graphics"},
    {0x1315, 0xD7, "AMD Radeon R5 Graphics"},
    {0x1316, 0x00, "AMD Radeon R5 Graphics"},
    {0x1318, 0x00, "AMD Radeon R5 Graphics"},
    {0x131B, 0x00, "AMD Radeon R4 Graphics"},
    {0x131C, 0x00, "AMD Radeon R7 Graphics"},
    {0x131D, 0x00, "AMD Radeon R6 Graphics"},
    {0x15D8, 0x00, "AMD Radeon RX Vega 8 Graphics WS"},
    {0x15D8, 0x91, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0x91, "AMD Ryzen Embedded R1606G with Radeon Vega Gfx"},
    {0x15D8, 0x92, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0x92, "AMD Ryzen Embedded R1505G with Radeon Vega Gfx"},
    {0x15D8, 0x93, "AMD Radeon Vega 1 Graphics"},
    {0x15D8, 0xA1, "AMD Radeon Vega 10 Graphics"},
    {0x15D8, 0xA2, "AMD Radeon Vega 8 Graphics"},
    {0x15D8, 0xA3, "AMD Radeon Vega 6 Graphics"},
    {0x15D8, 0xA4, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xB1, "AMD Radeon Vega 10 Graphics"},
    {0x15D8, 0xB2, "AMD Radeon Vega 8 Graphics"},
    {0x15D8, 0xB3, "AMD Radeon Vega 6 Graphics"},
    {0x15D8, 0xB4, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xC1, "AMD Radeon Vega 10 Graphics"},
    {0x15D8, 0xC2, "AMD Radeon Vega 8 Graphics"},
    {0x15D8, 0xC3, "AMD Radeon Vega 6 Graphics"},
    {0x15D8, 0xC4, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xC5, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xC8, "AMD Radeon Vega 11 Graphics"},
    {0x15D8, 0xC9, "AMD Radeon Vega 8 Graphics"},
    {0x15D8, 0xCA, "AMD Radeon Vega 11 Graphics"},
    {0x15D8, 0xCB, "AMD Radeon Vega 8 Graphics"},
    {0x15D8, 0xCC, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xCE, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xCF, "AMD Ryzen Embedded R1305G with Radeon Vega Gfx"},
    {0x15D8, 0xD1, "AMD Radeon Vega 10 Graphics"},
    {0x15D8, 0xD2, "AMD Radeon Vega 8 Graphics"},
    {0x15D8, 0xD3, "AMD Radeon Vega 6 Graphics"},
    {0x15D8, 0xD4, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xD8, "AMD Radeon Vega 11 Graphics"},
    {0x15D8, 0xD9, "AMD Radeon Vega 8 Graphics"},
    {0x15D8, 0xDA, "AMD Radeon Vega 11 Graphics"},
    {0x15D8, 0xDB, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xDB, "AMD Radeon Vega 8 Graphics"},
    {0x15D8, 0xDC, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xDD, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xDE, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xDF, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xE3, "AMD Radeon Vega 3 Graphics"},
    {0x15D8, 0xE4, "AMD Ryzen Embedded R1102G with Radeon Vega Gfx"},
    {0x15DD, 0x81, "AMD Ryzen Embedded V1807B with Radeon Vega Gfx"},
    {0x15DD, 0x82, "AMD Ryzen Embedded V1756B with Radeon Vega Gfx"},
    {0x15DD, 0x83, "AMD Ryzen Embedded V1605B with Radeon Vega Gfx"},
    {0x15DD, 0x84, "AMD Radeon Vega 6 Graphics"},
    {0x15DD, 0x85, "AMD Ryzen Embedded V1202B with Radeon Vega Gfx"},
    {0x15DD, 0x86, "AMD Radeon Vega 11 Graphics"},
    {0x15DD, 0x88, "AMD Radeon Vega 8 Graphics"},
    {0x15DD, 0xC1, "AMD Radeon Vega 11 Graphics"},
    {0x15DD, 0xC2, "AMD Radeon Vega 8 Graphics"},
    {0x15DD, 0xC3, "AMD Radeon Vega 3 / 10 Graphics"},
    {0x15DD, 0xC4, "AMD Radeon Vega 8 Graphics"},
    {0x15DD, 0xC5, "AMD Radeon Vega 3 Graphics"},
    {0x15DD, 0xC6, "AMD Radeon Vega 11 Graphics"},
    {0x15DD, 0xC8, "AMD Radeon Vega 8 Graphics"},
    {0x15DD, 0xC9, "AMD Radeon Vega 11 Graphics"},
    {0x15DD, 0xCA, "AMD Radeon Vega 8 Graphics"},
    {0x15DD, 0xCB, "AMD Radeon Vega 3 Graphics"},
    {0x15DD, 0xCC, "AMD Radeon Vega 6 Graphics"},
    {0x15DD, 0xCE, "AMD Radeon Vega 3 Graphics"},
    {0x15DD, 0xCF, "AMD Radeon Vega 3 Graphics"},
    {0x15DD, 0xD0, "AMD Radeon Vega 10 Graphics"},
    {0x15DD, 0xD1, "AMD Radeon Vega 8 Graphics"},
    {0x15DD, 0xD3, "AMD Radeon Vega 11 Graphics"},
    {0x15DD, 0xD5, "AMD Radeon Vega 8 Graphics"},
    {0x15DD, 0xD6, "AMD Radeon Vega 11 Graphics"},
    {0x15DD, 0xD7, "AMD Radeon Vega 8 Graphics"},
    {0x15DD, 0xD8, "AMD Radeon Vega 3 Graphics"},
    {0x15DD, 0xD9, "AMD Radeon Vega 6 Graphics"},
    {0x15DD, 0xE1, "AMD Radeon Vega 3 Graphics"},
    {0x15DD, 0xE2, "AMD Radeon Vega 3 Graphics"},
    {0x163F, 0xAE, "AMD Custom GPU 0405"},
    {0x6600, 0x00, "AMD Radeon HD 8600 / 8700M"},
    {0x6600, 0x81, "AMD Radeon R7 M370"},
    {0x6601, 0x00, "AMD Radeon HD 8500M / 8700M"},
    {0x6604, 0x00, "AMD Radeon R7 M265 Series"},
    {0x6604, 0x81, "AMD Radeon R7 M350"},
    {0x6605, 0x00, "AMD Radeon R7 M260 Series"},
    {0x6605, 0x81, "AMD Radeon R7 M340"},
    {0x6606, 0x00, "AMD Radeon HD 8790M"},
    {0x6607, 0x00, "AMD Radeon R5 M240"},
    {0x6608, 0x00, "AMD FirePro W2100"},
    {0x6610, 0x00, "AMD Radeon R7 200 Series"},
    {0x6610, 0x81, "AMD Radeon R7 350"},
    {0x6610, 0x83, "AMD Radeon R5 340"},
    {0x6610, 0x87, "AMD Radeon R7 200 Series"},
    {0x6611, 0x00, "AMD Radeon R7 200 Series"},
    {0x6611, 0x87, "AMD Radeon R7 200 Series"},
    {0x6613, 0x00, "AMD Radeon R7 200 Series"},
    {0x6617, 0x00, "AMD Radeon R7 240 Series"},
    {0x6617, 0x87, "AMD Radeon R7 200 Series"},
    {0x6617, 0xC7, "AMD Radeon R7 240 Series"},
    {0x6640, 0x00, "AMD Radeon HD 8950"},
    {0x6640, 0x80, "AMD Radeon R9 M380"},
    {0x6646, 0x00, "AMD Radeon R9 M280X"},
    {0x6646, 0x80, "AMD Radeon R9 M385"},
    {0x6646, 0x80, "AMD Radeon R9 M470X"},
    {0x6647, 0x00, "AMD Radeon R9 M200X Series"},
    {0x6647, 0x80, "AMD Radeon R9 M380"},
    {0x6649, 0x00, "AMD FirePro W5100"},
    {0x6658, 0x00, "AMD Radeon R7 200 Series"},
    {0x665C, 0x00, "AMD Radeon HD 7700 Series"},
    {0x665D, 0x00, "AMD Radeon R7 200 Series"},
    {0x665F, 0x81, "AMD Radeon R7 360 Series"},
    {0x6660, 0x00, "AMD Radeon HD 8600M Series"},
    {0x6660, 0x81, "AMD Radeon R5 M335"},
    {0x6660, 0x83, "AMD Radeon R5 M330"},
    {0x6663, 0x00, "AMD Radeon HD 8500M Series"},
    {0x6663, 0x83, "AMD Radeon R5 M320"},
    {0x6664, 0x00, "AMD Radeon R5 M200 Series"},
    {0x6665, 0x00, "AMD Radeon R5 M230 Series"},
    {0x6665, 0x83, "AMD Radeon R5 M320"},
    {0x6665, 0xC3, "AMD Radeon R5 M435"},
    {0x6666, 0x00, "AMD Radeon R5 M200 Series"},
    {0x6667, 0x00, "AMD Radeon R5 M200 Series"},
    {0x666F, 0x00, "AMD Radeon HD 8500M"},
    {0x66A1, 0x02, "AMD Instinct MI60 / MI50"},
    {0x66A1, 0x06, "AMD Radeon Pro VII"},
    {0x66AF, 0xC1, "AMD Radeon VII"},
    {0x6780, 0x00, "AMD FirePro W9000"},
    {0x6784, 0x00, "ATI FirePro V (FireGL V) Graphics Adapter"},
    {0x6788, 0x00, "ATI FirePro V (FireGL V) Graphics Adapter"},
    {0x678A, 0x00, "AMD FirePro W8000"},
    {0x6798, 0x00, "AMD Radeon R9 200 / HD 7900 Series"},
    {0x6799, 0x00, "AMD Radeon HD 7900 Series"},
    {0x679A, 0x00, "AMD Radeon HD 7900 Series"},
    {0x679B, 0x00, "AMD Radeon HD 7900 Series"},
    {0x679E, 0x00, "AMD Radeon HD 7800 Series"},
    {0x67A0, 0x00, "AMD Radeon FirePro W9100"},
    {0x67A1, 0x00, "AMD Radeon FirePro W8100"},
    {0x67B0, 0x00, "AMD Radeon R9 200 Series"},
    {0x67B0, 0x80, "AMD Radeon R9 390 Series"},
    {0x67B1, 0x00, "AMD Radeon R9 200 Series"},
    {0x67B1, 0x80, "AMD Radeon R9 390 Series"},
    {0x67B9, 0x00, "AMD Radeon R9 200 Series"},
    {0x67C0, 0x00, "AMD Radeon Pro WX 7100 Graphics"},
    {0x67C0, 0x80, "AMD Radeon E9550"},
    {0x67C2, 0x01, "AMD Radeon Pro V7350x2"},
    {0x67C2, 0x02, "AMD Radeon Pro V7300X"},
    {0x67C4, 0x00, "AMD Radeon Pro WX 7100 Graphics"},
    {0x67C4, 0x80, "AMD Radeon E9560 / E9565 Graphics"},
    {0x67C7, 0x00, "AMD Radeon Pro WX 5100 Graphics"},
    {0x67C7, 0x80, "AMD Radeon E9390 Graphics"},
    {0x67D0, 0x01, "AMD Radeon Pro V7350x2"},
    {0x67D0, 0x02, "AMD Radeon Pro V7300X"},
    {0x67DF, 0xC0, "AMD Radeon Pro 580X"},
    {0x67DF, 0xC1, "AMD Radeon RX 580 Series"},
    {0x67DF, 0xC2, "AMD Radeon RX 570 Series"},
    {0x67DF, 0xC3, "AMD Radeon RX 580 Series"},
    {0x67DF, 0xC4, "AMD Radeon RX 480 Graphics"},
    {0x67DF, 0xC5, "AMD Radeon RX 470 Graphics"},
    {0x67DF, 0xC6, "AMD Radeon RX 570 Series"},
    {0x67DF, 0xC7, "AMD Radeon RX 480 Graphics"},
    {0x67DF, 0xCF, "AMD Radeon RX 470 Graphics"},
    {0x67DF, 0xD7, "AMD Radeon RX 470 Graphics"},
    {0x67DF, 0xE0, "AMD Radeon RX 470 Series"},
    {0x67DF, 0xE1, "AMD Radeon RX 590 Series"},
    {0x67DF, 0xE3, "AMD Radeon RX Series"},
    {0x67DF, 0xE7, "AMD Radeon RX 580 Series"},
    {0x67DF, 0xEB, "AMD Radeon Pro 580X"},
    {0x67DF, 0xEF, "AMD Radeon RX 570 Series"},
    {0x67DF, 0xF7, "AMD Radeon RX P30PH"},
    {0x67DF, 0xFF, "AMD Radeon RX 470 Series"},
    {0x67E0, 0x00, "AMD Radeon Pro WX Series"},
    {0x67E3, 0x00, "AMD Radeon Pro WX 4100"},
    {0x67E8, 0x00, "AMD Radeon Pro WX Series"},
    {0x67E8, 0x01, "AMD Radeon Pro WX Series"},
    {0x67E8, 0x80, "AMD Radeon E9260 Graphics"},
    {0x67EB, 0x00, "AMD Radeon Pro V5300X"},
    {0x67EF, 0xC0, "AMD Radeon RX Graphics"},
    {0x67EF, 0xC1, "AMD Radeon RX 460 Graphics"},
    {0x67EF, 0xC2, "AMD Radeon Pro Series"},
    {0x67EF, 0xC3, "AMD Radeon RX Series"},
    {0x67EF, 0xC5, "AMD Radeon RX 460 Graphics"},
    {0x67EF, 0xC7, "AMD Radeon RX Graphics"},
    {0x67EF, 0xCF, "AMD Radeon RX 460 Graphics"},
    {0x67EF, 0xE0, "AMD Radeon RX 560 Series"},
    {0x67EF, 0xE1, "AMD Radeon RX Series"},
    {0x67EF, 0xE2, "AMD Radeon RX 560X"},
    {0x67EF, 0xE3, "AMD Radeon RX Series"},
    {0x67EF, 0xE5, "AMD Radeon RX 560 Series"},
    {0x67EF, 0xE7, "AMD Radeon RX 560 Series"},
    {0x67EF, 0xEF, "AMD Radeon 550 Series"},
    {0x67EF, 0xFF, "AMD Radeon RX 460 Graphics"},
    {0x67FF, 0xC0, "AMD Radeon Pro 465"},
    {0x67FF, 0xC1, "AMD Radeon RX 560 Series"},
    {0x67FF, 0xCF, "AMD Radeon RX 560 Series"},
    {0x67FF, 0xEF, "AMD Radeon RX 560 Series"},
    {0x67FF, 0xFF, "AMD Radeon RX 550 Series"},
    {0x6800, 0x00, "AMD Radeon HD 7970M"},
    {0x6801, 0x00, "AMD Radeon HD 8970M"},
    {0x6806, 0x00, "AMD Radeon R9 M290X"},
    {0x6808, 0x00, "AMD FirePro W7000"},
    {0x6808, 0x00, "ATI FirePro V (FireGL V) Graphics Adapter"},
    {0x6809, 0x00, "ATI FirePro W5000"},
    {0x6810, 0x00, "AMD Radeon R9 200 Series"},
    {0x6810, 0x81, "AMD Radeon R9 370 Series"},
    {0x6811, 0x00, "AMD Radeon R9 200 Series"},
    {0x6811, 0x81, "AMD Radeon R7 370 Series"},
    {0x6818, 0x00, "AMD Radeon HD 7800 Series"},
    {0x6819, 0x00, "AMD Radeon HD 7800 Series"},
    {0x6820, 0x00, "AMD Radeon R9 M275X"},
    {0x6820, 0x81, "AMD Radeon R9 M375"},
    {0x6820, 0x83, "AMD Radeon R9 M375X"},
    {0x6821, 0x00, "AMD Radeon R9 M200X Series"},
    {0x6821, 0x83, "AMD Radeon R9 M370X"},
    {0x6821, 0x87, "AMD Radeon R7 M380"},
    {0x6822, 0x00, "AMD Radeon E8860"},
    {0x6823, 0x00, "AMD Radeon R9 M200X Series"},
    {0x6825, 0x00, "AMD Radeon HD 7800M Series"},
    {0x6826, 0x00, "AMD Radeon HD 7700M Series"},
    {0x6827, 0x00, "AMD Radeon HD 7800M Series"},
    {0x6828, 0x00, "AMD FirePro W600"},
    {0x682B, 0x00, "AMD Radeon HD 8800M Series"},
    {0x682B, 0x87, "AMD Radeon R9 M360"},
    {0x682C, 0x00, "AMD FirePro W4100"},
    {0x682D, 0x00, "AMD Radeon HD 7700M Series"},
    {0x682F, 0x00, "AMD Radeon HD 7700M Series"},
    {0x6830, 0x00, "AMD Radeon 7800M Series"},
    {0x6831, 0x00, "AMD Radeon 7700M Series"},
    {0x6835, 0x00, "AMD Radeon R7 Series / HD 9000 Series"},
    {0x6837, 0x00, "AMD Radeon HD 7700 Series"},
    {0x683D, 0x00, "AMD Radeon HD 7700 Series"},
    {0x683F, 0x00, "AMD Radeon HD 7700 Series"},
    {0x684C, 0x00, "ATI FirePro V (FireGL V) Graphics Adapter"},
    {0x6860, 0x00, "AMD Radeon Instinct MI25"},
    {0x6860, 0x01, "AMD Radeon Instinct MI25"},
    {0x6860, 0x02, "AMD Radeon Instinct MI25"},
    {0x6860, 0x03, "AMD Radeon Pro V340"},
    {0x6860, 0x04, "AMD Radeon Instinct MI25x2"},
    {0x6860, 0x07, "AMD Radeon Pro V320"},
    {0x6861, 0x00, "AMD Radeon Pro WX 9100"},
    {0x6862, 0x00, "AMD Radeon Pro SSG"},
    {0x6863, 0x00, "AMD Radeon Vega Frontier Edition"},
    {0x6864, 0x03, "AMD Radeon Pro V340"},
    {0x6864, 0x04, "AMD Radeon Instinct MI25x2"},
    {0x6864, 0x05, "AMD Radeon Pro V340"},
    {0x6868, 0x00, "AMD Radeon Pro WX 8200"},
    {0x686C, 0x00, "AMD Radeon Instinct MI25 MxGPU"},
    {0x686C, 0x01, "AMD Radeon Instinct MI25 MxGPU"},
    {0x686C, 0x02, "AMD Radeon Instinct MI25 MxGPU"},
    {0x686C, 0x03, "AMD Radeon Pro V340 MxGPU"},
    {0x686C, 0x04, "AMD Radeon Instinct MI25x2 MxGPU"},
    {0x686C, 0x05, "AMD Radeon Pro V340L MxGPU"},
    {0x686C, 0x06, "AMD Radeon Instinct MI25 MxGPU"},
    {0x687F, 0x01, "AMD Radeon RX Vega"},
    {0x687F, 0xC0, "AMD Radeon RX Vega"},
    {0x687F, 0xC1, "AMD Radeon RX Vega"},
    {0x687F, 0xC3, "AMD Radeon RX Vega"},
    {0x687F, 0xC7, "AMD Radeon RX Vega"},
    {0x6900, 0x00, "AMD Radeon R7 M260"},
    {0x6900, 0x81, "AMD Radeon R7 M360"},
    {0x6900, 0x83, "AMD Radeon R7 M340"},
    {0x6900, 0xC1, "AMD Radeon R5 M465 Series"},
    {0x6900, 0xC3, "AMD Radeon R5 M445 Series"},
    {0x6900, 0xD1, "AMD Radeon 530 Series"},
    {0x6900, 0xD3, "AMD Radeon 530 Series"},
    {0x6901, 0x00, "AMD Radeon R5 M255"},
    {0x6902, 0x00, "AMD Radeon Series"},
    {0x6907, 0x00, "AMD Radeon R5 M255"},
    {0x6907, 0x87, "AMD Radeon R5 M315"},
    {0x6920, 0x00, "AMD Radeon R9 M395X"},
    {0x6920, 0x01, "AMD Radeon R9 M390X"},
    {0x6921, 0x00, "AMD Radeon R9 M390X"},
    {0x6929, 0x00, "AMD FirePro S7150"},
    {0x6929, 0x01, "AMD FirePro S7100X"},
    {0x692B, 0x00, "AMD FirePro W7100"},
    {0x6938, 0x00, "AMD Radeon R9 200 Series"},
    {0x6938, 0xF0, "AMD Radeon R9 200 Series"},
    {0x6938, 0xF1, "AMD Radeon R9 380 Series"},
    {0x6939, 0x00, "AMD Radeon R9 200 Series"},
    {0x6939, 0xF0, "AMD Radeon R9 200 Series"},
    {0x6939, 0xF1, "AMD Radeon R9 380 Series"},
    {0x694C, 0xC0, "AMD Radeon RX Vega M GH Graphics"},
    {0x694E, 0xC0, "AMD Radeon RX Vega M GL Graphics"},
    {0x6980, 0x00, "AMD Radeon Pro WX 3100"},
    {0x6981, 0x00, "AMD Radeon Pro WX 3200 Series"},
    {0x6981, 0x01, "AMD Radeon Pro WX 3200 Series"},
    {0x6981, 0x10, "AMD Radeon Pro WX 3200 Series"},
    {0x6985, 0x00, "AMD Radeon Pro WX 3100"},
    {0x6986, 0x00, "AMD Radeon Pro WX 2100"},
    {0x6987, 0x80, "AMD Embedded Radeon E9171"},
    {0x6987, 0xC0, "AMD Radeon 550X Series"},
    {0x6987, 0xC1, "AMD Radeon RX 640"},
    {0x6987, 0xC3, "AMD Radeon 540X Series"},
    {0x6987, 0xC7, "AMD Radeon 540"},
    {0x6995, 0x00, "AMD Radeon Pro WX 2100"},
    {0x6997, 0x00, "AMD Radeon Pro WX 2100"},
    {0x699F, 0x81, "AMD Embedded Radeon E9170 Series"},
    {0x699F, 0xC0, "AMD Radeon 500 Series"},
    {0x699F, 0xC1, "AMD Radeon 540 Series"},
    {0x699F, 0xC3, "AMD Radeon 500 Series"},
    {0x699F, 0xC7, "AMD Radeon RX 550 / 550 Series"},
    {0x699F, 0xC9, "AMD Radeon 540"},
    {0x6FDF, 0xE7, "AMD Radeon RX 590 GME"},
    {0x6FDF, 0xEF, "AMD Radeon RX 580 2048SP"},
    {0x7300, 0xC1, "AMD FirePro S9300 x2"},
    {0x7300, 0xC8, "AMD Radeon R9 Fury Series"},
    {0x7300, 0xC9, "AMD Radeon Pro Duo"},
    {0x7300, 0xCA, "AMD Radeon R9 Fury Series"},
    {0x7300, 0xCB, "AMD Radeon R9 Fury Series"},
    {0x7312, 0x00, "AMD Radeon Pro W5700"},
    {0x731E, 0xC6, "AMD Radeon RX 5700XTB"},
    {0x731E, 0xC7, "AMD Radeon RX 5700B"},
    {0x731F, 0xC0, "AMD Radeon RX 5700 XT 50th Anniversary"},
    {0x731F, 0xC1, "AMD Radeon RX 5700 XT"},
    {0x731F, 0xC2, "AMD Radeon RX 5600M"},
    {0x731F, 0xC3, "AMD Radeon RX 5700M"},
    {0x731F, 0xC4, "AMD Radeon RX 5700"},
    {0x731F, 0xC5, "AMD Radeon RX 5700 XT"},
    {0x731F, 0xCA, "AMD Radeon RX 5600 XT"},
    {0x731F, 0xCB, "AMD Radeon RX 5600 OEM"},
    {0x7340, 0xC1, "AMD Radeon RX 5500M"},
    {0x7340, 0xC3, "AMD Radeon RX 5300M"},
    {0x7340, 0xC5, "AMD Radeon RX 5500 XT"},
    {0x7340, 0xC7, "AMD Radeon RX 5500"},
    {0x7340, 0xC9, "AMD Radeon RX 5500XTB"},
    {0x7340, 0xCF, "AMD Radeon RX 5300"},
    {0x7341, 0x00, "AMD Radeon Pro W5500"},
    {0x7347, 0x00, "AMD Radeon Pro W5500M"},
    {0x7360, 0x41, "AMD Radeon Pro 5600M"},
    {0x7360, 0xC3, "AMD Radeon Pro V520"},
    {0x738C, 0x01, "AMD Instinct MI100"},
    {0x73A3, 0x00, "AMD Radeon Pro W6800"},
    {0x73A5, 0xC0, "AMD Radeon RX 6950 XT"},
    {0x73AF, 0xC0, "AMD Radeon RX 6900 XT"},
    {0x73BF, 0xC0, "AMD Radeon RX 6900 XT"},
    {0x73BF, 0xC1, "AMD Radeon RX 6800 XT"},
    {0x73BF, 0xC3, "AMD Radeon RX 6800"},
    {0x73DF, 0xC0, "AMD Radeon RX 6750 XT"},
    {0x73DF, 0xC1, "AMD Radeon RX 6700 XT"},
    {0x73DF, 0xC2, "AMD Radeon RX 6800M"},
    {0x73DF, 0xC3, "AMD Radeon RX 6800M"},
    {0x73DF, 0xC5, "AMD Radeon RX 6700 XT"},
    {0x73DF, 0xCF, "AMD Radeon RX 6700M"},
    {0x73DF, 0xD7, "AMD TDC-235"},
    {0x73E1, 0x00, "AMD Radeon Pro W6600M"},
    {0x73E3, 0x00, "AMD Radeon Pro W6600"},
    {0x73EF, 0xC0, "AMD Radeon RX 6800S"},
    {0x73EF, 0xC1, "AMD Radeon RX 6650 XT"},
    {0x73EF, 0xC2, "AMD Radeon RX 6700S"},
    {0x73EF, 0xC3, "AMD Radeon RX 6650M"},
    {0x73EF, 0xC4, "AMD Radeon RX 6650M XT"},
    {0x73FF, 0xC1, "AMD Radeon RX 6600 XT"},
    {0x73FF, 0xC3, "AMD Radeon RX 6600M"},
    {0x73FF, 0xC7, "AMD Radeon RX 6600"},
    {0x73FF, 0xCB, "AMD Radeon RX 6600S"},
    {0x7408, 0x00, "AMD Instinct MI250X"},
    {0x740C, 0x01, "AMD Instinct MI250X / MI250"},
    {0x740F, 0x02, "AMD Instinct MI210"},
    {0x7421, 0x00, "AMD Radeon Pro W6500M"},
    {0x7422, 0x00, "AMD Radeon Pro W6400"},
    {0x7423, 0x00, "AMD Radeon Pro W6300M"},
    {0x7423, 0x01, "AMD Radeon Pro W6300"},
    {0x7424, 0x00, "AMD Radeon RX 6300"},
    {0x743F, 0xC1, "AMD Radeon RX 6500 XT"},
    {0x743F, 0xC3, "AMD Radeon RX 6500"},
    {0x743F, 0xC3, "AMD Radeon RX 6500M"},
    {0x743F, 0xC7, "AMD Radeon RX 6400"},
    {0x743F, 0xCF, "AMD Radeon RX 6300M"},
    {0x744C, 0xC8, "AMD Radeon RX 7900 XTX"},
    {0x744C, 0xCC, "AMD Radeon RX 7900 XT"},
    {0x7480, 0xC1, "AMD Radeon RX 7700S"},
    {0x7480, 0xC3, "AMD Radeon RX 7600S"},
    {0x7480, 0xC7, "AMD Radeon RX 7600M XT"},
    {0x7483, 0xCF, "AMD Radeon RX 7600M"},
    {0x9830, 0x00, "AMD Radeon HD 8400 / R3 Series"},
    {0x9831, 0x00, "AMD Radeon HD 8400E"},
    {0x9832, 0x00, "AMD Radeon HD 8330"},
    {0x9833, 0x00, "AMD Radeon HD 8330E"},
    {0x9834, 0x00, "AMD Radeon HD 8210"},
    {0x9835, 0x00, "AMD Radeon HD 8210E"},
    {0x9836, 0x00, "AMD Radeon HD 8200 / R3 Series"},
    {0x9837, 0x00, "AMD Radeon HD 8280E"},
    {0x9838, 0x00, "AMD Radeon HD 8200 / R3 series"},
    {0x9839, 0x00, "AMD Radeon HD 8180"},
    {0x983D, 0x00, "AMD Radeon HD 8250"},
    {0x9850, 0x00, "AMD Radeon R3 Graphics"},
    {0x9850, 0x03, "AMD Radeon R3 Graphics"},
    {0x9850, 0x40, "AMD Radeon R2 Graphics"},
    {0x9850, 0x45, "AMD Radeon R3 Graphics"},
    {0x9851, 0x00, "AMD Radeon R4 Graphics"},
    {0x9851, 0x01, "AMD Radeon R5E Graphics"},
    {0x9851, 0x05, "AMD Radeon R5 Graphics"},
    {0x9851, 0x06, "AMD Radeon R5E Graphics"},
    {0x9851, 0x40, "AMD Radeon R4 Graphics"},
    {0x9851, 0x45, "AMD Radeon R5 Graphics"},
    {0x9852, 0x00, "AMD Radeon R2 Graphics"},
    {0x9852, 0x40, "AMD Radeon E1 Graphics"},
    {0x9853, 0x00, "AMD Radeon R2 Graphics"},
    {0x9853, 0x01, "AMD Radeon R4E Graphics"},
    {0x9853, 0x03, "AMD Radeon R2 Graphics"},
    {0x9853, 0x05, "AMD Radeon R1E Graphics"},
    {0x9853, 0x06, "AMD Radeon R1E Graphics"},
    {0x9853, 0x07, "AMD Radeon R1E Graphics"},
    {0x9853, 0x08, "AMD Radeon R1E Graphics"},
    {0x9853, 0x40, "AMD Radeon R2 Graphics"},
    {0x9854, 0x00, "AMD Radeon R3 Graphics"},
    {0x9854, 0x01, "AMD Radeon R3E Graphics"},
    {0x9854, 0x02, "AMD Radeon R3 Graphics"},
    {0x9854, 0x05, "AMD Radeon R2 Graphics"},
    {0x9854, 0x06, "AMD Radeon R4 Graphics"},
    {0x9854, 0x07, "AMD Radeon R3 Graphics"},
    {0x9855, 0x02, "AMD Radeon R6 Graphics"},
    {0x9855, 0x05, "AMD Radeon R4 Graphics"},
    {0x9856, 0x00, "AMD Radeon R2 Graphics"},
    {0x9856, 0x01, "AMD Radeon R2E Graphics"},
    {0x9856, 0x02, "AMD Radeon R2 Graphics"},
    {0x9856, 0x05, "AMD Radeon R1E Graphics"},
    {0x9856, 0x06, "AMD Radeon R2 Graphics"},
    {0x9856, 0x07, "AMD Radeon R1E Graphics"},
    {0x9856, 0x08, "AMD Radeon R1E Graphics"},
    {0x9856, 0x13, "AMD Radeon R1E Graphics"},
    {0x9874, 0x81, "AMD Radeon R6 Graphics"},
    {0x9874, 0x84, "AMD Radeon R7 Graphics"},
    {0x9874, 0x85, "AMD Radeon R6 Graphics"},
    {0x9874, 0x87, "AMD Radeon R5 Graphics"},
    {0x9874, 0x88, "AMD Radeon R7E Graphics"},
    {0x9874, 0x89, "AMD Radeon R6E Graphics"},
    {0x9874, 0xC4, "AMD Radeon R7 Graphics"},
    {0x9874, 0xC5, "AMD Radeon R6 Graphics"},
    {0x9874, 0xC6, "AMD Radeon R6 Graphics"},
    {0x9874, 0xC7, "AMD Radeon R5 Graphics"},
    {0x9874, 0xC8, "AMD Radeon R7 Graphics"},
    {0x9874, 0xC9, "AMD Radeon R7 Graphics"},
    {0x9874, 0xCA, "AMD Radeon R5 Graphics"},
    {0x9874, 0xCB, "AMD Radeon R5 Graphics"},
    {0x9874, 0xCC, "AMD Radeon R7 Graphics"},
    {0x9874, 0xCD, "AMD Radeon R7 Graphics"},
    {0x9874, 0xCE, "AMD Radeon R5 Graphics"},
    {0x9874, 0xE1, "AMD Radeon R7 Graphics"},
    {0x9874, 0xE2, "AMD Radeon R7 Graphics"},
    {0x9874, 0xE3, "AMD Radeon R7 Graphics"},
    {0x9874, 0xE4, "AMD Radeon R7 Graphics"},
    {0x9874, 0xE5, "AMD Radeon R5 Graphics"},
    {0x9874, 0xE6, "AMD Radeon R5 Graphics"},
    {0x98E4, 0x80, "AMD Radeon R5E Graphics"},
    {0x98E4, 0x81, "AMD Radeon R4E Graphics"},
    {0x98E4, 0x83, "AMD Radeon R2E Graphics"},
    {0x98E4, 0x84, "AMD Radeon R2E Graphics"},
    {0x98E4, 0x86, "AMD Radeon R1E Graphics"},
    {0x98E4, 0xC0, "AMD Radeon R4 Graphics"},
    {0x98E4, 0xC1, "AMD Radeon R5 Graphics"},
    {0x98E4, 0xC2, "AMD Radeon R4 Graphics"},
    {0x98E4, 0xC4, "AMD Radeon R5 Graphics"},
    {0x98E4, 0xC6, "AMD Radeon R5 Graphics"},
    {0x98E4, 0xC8, "AMD Radeon R4 Graphics"},
    {0x98E4, 0xC9, "AMD Radeon R4 Graphics"},
    {0x98E4, 0xCA, "AMD Radeon R5 Graphics"},
    {0x98E4, 0xD0, "AMD Radeon R2 Graphics"},
    {0x98E4, 0xD1, "AMD Radeon R2 Graphics"},
    {0x98E4, 0xD2, "AMD Radeon R2 Graphics"},
    {0x98E4, 0xD4, "AMD Radeon R2 Graphics"},
    {0x98E4, 0xD9, "AMD Radeon R5 Graphics"},
    {0x98E4, 0xDA, "AMD Radeon R5 Graphics"},
    {0x98E4, 0xDB, "AMD Radeon R3 Graphics"},
    {0x98E4, 0xE1, "AMD Radeon R3 Graphics"},
    {0x98E4, 0xE2, "AMD Radeon R3 Graphics"},
    {0x98E4, 0xE9, "AMD Radeon R4 Graphics"},
    {0x98E4, 0xEA, "AMD Radeon R4 Graphics"},
    {0x98E4, 0xEB, "AMD Radeon R3 Graphics"},
    {0x98E4, 0xEC, "AMD Radeon R4 Graphics"},
    {0x0000, 0x00, "unknown AMD GPU"} // this must always be the last item
};

struct card {
    const char *pathname;
    struct amdgpu_id_struct id;

    /* GPU and VRAM utilizations */

    const char *pathname_util_gpu;
    RRDSET  *st_util_gpu;
    RRDDIM  *rd_util_gpu;
    collected_number util_gpu;

    const char *pathname_util_mem;
    RRDSET  *st_util_mem;
    RRDDIM  *rd_util_mem;
    collected_number util_mem;


    /* GPU and VRAM clock frequencies */

    const char *pathname_clk_gpu;
    procfile *ff_clk_gpu;
    RRDSET *st_clk_gpu;
    RRDDIM *rd_clk_gpu;
    collected_number clk_gpu;

    const char *pathname_clk_mem;
    procfile *ff_clk_mem;
    RRDSET *st_clk_mem;
    RRDDIM *rd_clk_mem;
    collected_number clk_mem;


    /* GPU memory usage */

    const char *pathname_mem_used_vram;
    const char *pathname_mem_total_vram;

    RRDSET  *st_mem_usage_perc_vram;
    RRDDIM  *rd_mem_used_perc_vram;

    RRDSET  *st_mem_usage_vram;
    RRDDIM  *rd_mem_used_vram;
    RRDDIM  *rd_mem_free_vram;

    collected_number used_vram;
    collected_number total_vram;


    const char *pathname_mem_used_vis_vram;
    const char *pathname_mem_total_vis_vram;

    RRDSET  *st_mem_usage_perc_vis_vram;
    RRDDIM  *rd_mem_used_perc_vis_vram;

    RRDSET  *st_mem_usage_vis_vram;
    RRDDIM  *rd_mem_used_vis_vram;
    RRDDIM  *rd_mem_free_vis_vram;

    collected_number used_vis_vram;
    collected_number total_vis_vram;


    const char *pathname_mem_used_gtt;
    const char *pathname_mem_total_gtt;

    RRDSET  *st_mem_usage_perc_gtt;
    RRDDIM  *rd_mem_used_perc_gtt;

    RRDSET  *st_mem_usage_gtt;
    RRDDIM  *rd_mem_used_gtt;
    RRDDIM  *rd_mem_free_gtt;
    
    collected_number used_gtt;
    collected_number total_gtt;
    
    struct do_rrd_x *do_rrd_x_root;
    
    struct card *next;
};
static struct card *card_root = NULL;

static void card_free(struct card *c){
    if(c->pathname) freez((void *) c->pathname);
    if(c->id.marketing_name) freez((void *) c->id.marketing_name);

    /* remove card from linked list */
    if(c == card_root) card_root = c->next;
    else {
        struct card *last;
        for(last = card_root; last && last->next != c; last = last->next);
        if(last) last->next = c->next;
    }

    freez(c);
}

static int check_card_is_amdgpu(const char *const pathname){
    int rc = -1;

    procfile *ff = procfile_open(pathname, " ", PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
    if(unlikely(!ff)){
        rc = -1;
        goto cleanup;
    } 

    ff = procfile_readall(ff);
    if(unlikely(!ff || procfile_lines(ff) < 1 || procfile_linewords(ff, 0) < 1)){
        rc = -2;
        goto cleanup;
    }

    for(size_t l = 0; l < procfile_lines(ff); l++) {
        if(!strcmp(procfile_lineword(ff, l, 0), "DRIVER=amdgpu")){
            rc = 0;
            goto cleanup;
        }
    }

    rc = -3; // no match

cleanup:
    procfile_close(ff);
    return rc;
}

static int read_clk_freq_file(procfile **p_ff, const char *const pathname, collected_number *num){
    if(unlikely(!*p_ff)){
        *p_ff = procfile_open(pathname, NULL, PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
        if(unlikely(!*p_ff))
            return -1;
    }

    if(unlikely(NULL == (*p_ff = procfile_readall(*p_ff))))
        return -1;

    for(size_t l = 0; l < procfile_lines(*p_ff) ; l++) {
        char *str_with_units = NULL;

        if(procfile_linewords(*p_ff, l) >= 3 && !strcmp(procfile_lineword((*p_ff), l, 2), "*")) //format: X: collected_number *
            str_with_units = procfile_lineword((*p_ff), l, 1);
        else if (procfile_linewords(*p_ff, l) == 2 && !strcmp(procfile_lineword((*p_ff), l, 1), "*")) //format:   collected_number *
            str_with_units = procfile_lineword((*p_ff), l, 0);

        if (str_with_units) {
            char *units = NULL;
            *num = str2ll(str_with_units, &units);
            return 0;
        }
    }

    *num = 0; // the card is not active, so no speed reporting
    return 0;
}

static char *set_id(const char *const suf_1, const char *const suf_2, const char *const suf_3){
    static char id[RRD_ID_LENGTH_MAX + 1];
    snprintfz(id, RRD_ID_LENGTH_MAX, "%s_%s_%s", suf_1, suf_2, suf_3);
    return id;
}

typedef int (*do_rrd_x_func)(struct card *const c);

struct do_rrd_x {
    do_rrd_x_func func;
    struct do_rrd_x *next;
};

static void add_do_rrd_x(struct card *const c, const do_rrd_x_func func){
    struct do_rrd_x *const drrd = callocz(1, sizeof(struct do_rrd_x));
    drrd->func = func;
    drrd->next = c->do_rrd_x_root;
    c->do_rrd_x_root = drrd;
}

static void rm_do_rrd_x(struct card *const c, struct do_rrd_x *const drrd){
    if(drrd == c->do_rrd_x_root) c->do_rrd_x_root = drrd->next;
    else {
        struct do_rrd_x *last;
        for(last = c->do_rrd_x_root; last && last->next != drrd; last = last->next);
        if(last) last->next = drrd->next;
    }

    freez(drrd);
}

static int do_rrd_util_gpu(struct card *const c){
    if(likely(!read_single_number_file(c->pathname_util_gpu, (unsigned long long *) &c->util_gpu))){
        rrddim_set_by_pointer(c->st_util_gpu, c->rd_util_gpu, c->util_gpu);
        rrdset_done(c->st_util_gpu);
        return 0;
    }
    else {
        collector_error("Cannot read util_gpu for %s: [%s]", c->pathname, c->id.marketing_name);
        freez((void *) c->pathname_util_gpu);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_util_gpu);
        return 1;
    }
}

static int do_rrd_util_mem(struct card *const c){
    if(likely(!read_single_number_file(c->pathname_util_mem, (unsigned long long *) &c->util_mem))){
        rrddim_set_by_pointer(c->st_util_mem, c->rd_util_mem, c->util_mem);
        rrdset_done(c->st_util_mem);
        return 0;
    }
    else {
        collector_error("Cannot read util_mem for %s: [%s]", c->pathname, c->id.marketing_name);
        freez((void *) c->pathname_util_mem);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_util_mem);
        return 1;
    }
}

static int do_rrd_clk_gpu(struct card *const c){
    if(likely(!read_clk_freq_file(&c->ff_clk_gpu, (char *) c->pathname_clk_gpu, &c->clk_gpu))){
        rrddim_set_by_pointer(c->st_clk_gpu, c->rd_clk_gpu, c->clk_gpu);
        rrdset_done(c->st_clk_gpu);
        return 0;
    }
    else {
        collector_error("Cannot read clk_gpu for %s: [%s]", c->pathname, c->id.marketing_name);
        freez((void *) c->pathname_clk_gpu);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_clk_gpu);
        return 1;
    }
}

static int do_rrd_clk_mem(struct card *const c){
    if(likely(!read_clk_freq_file(&c->ff_clk_mem, (char *) c->pathname_clk_mem, &c->clk_mem))){
        rrddim_set_by_pointer(c->st_clk_mem, c->rd_clk_mem, c->clk_mem);
        rrdset_done(c->st_clk_mem);
        return 0;
    }
    else {
        collector_error("Cannot read clk_mem for %s: [%s]", c->pathname, c->id.marketing_name);
        freez((void *) c->pathname_clk_mem);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_clk_mem);
        return 1;
    }
}

static int do_rrd_vram(struct card *const c){
    if(likely(!read_single_number_file(c->pathname_mem_used_vram, (unsigned long long *) &c->used_vram) && 
            c->total_vram)){
        rrddim_set_by_pointer(  c->st_mem_usage_perc_vram, 
                                c->rd_mem_used_perc_vram, 
                                c->used_vram * 10000 / c->total_vram);
        rrdset_done(c->st_mem_usage_perc_vram);

        rrddim_set_by_pointer(c->st_mem_usage_vram, c->rd_mem_used_vram, c->used_vram);
        rrddim_set_by_pointer(c->st_mem_usage_vram, c->rd_mem_free_vram, c->total_vram - c->used_vram);
        rrdset_done(c->st_mem_usage_vram);
        return 0;
    }
    else {
        collector_error("Cannot read used_vram for %s: [%s]", c->pathname, c->id.marketing_name);
        freez((void *) c->pathname_mem_used_vram);
        freez((void *) c->pathname_mem_total_vram);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_mem_usage_perc_vram);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_mem_usage_vram);
        return 1;
    }
}

static int do_rrd_vis_vram(struct card *const c){
    if(likely(!read_single_number_file(c->pathname_mem_used_vis_vram, (unsigned long long *) &c->used_vis_vram) && 
            c->total_vis_vram)){
        rrddim_set_by_pointer(  c->st_mem_usage_perc_vis_vram, 
                                c->rd_mem_used_perc_vis_vram, 
                                c->used_vis_vram * 10000 / c->total_vis_vram);
        rrdset_done(c->st_mem_usage_perc_vis_vram);
    
        rrddim_set_by_pointer(c->st_mem_usage_vis_vram, c->rd_mem_used_vis_vram, c->used_vis_vram);
        rrddim_set_by_pointer(c->st_mem_usage_vis_vram, c->rd_mem_free_vis_vram, c->total_vis_vram - c->used_vis_vram);
        rrdset_done(c->st_mem_usage_vis_vram);
        return 0;
    }
    else {
        collector_error("Cannot read used_vis_vram for %s: [%s]", c->pathname, c->id.marketing_name);
        freez((void *) c->pathname_mem_used_vis_vram);
        freez((void *) c->pathname_mem_total_vis_vram);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_mem_usage_perc_vis_vram);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_mem_usage_vis_vram);
        return 1;
    }
}

static int do_rrd_gtt(struct card *const c){
    if(likely(!read_single_number_file(c->pathname_mem_used_gtt, (unsigned long long *) &c->used_gtt) && 
            c->total_gtt)){
        rrddim_set_by_pointer(  c->st_mem_usage_perc_gtt, 
                                c->rd_mem_used_perc_gtt, 
                                c->used_gtt * 10000 / c->total_gtt);
        rrdset_done(c->st_mem_usage_perc_gtt);
    
        rrddim_set_by_pointer(c->st_mem_usage_gtt, c->rd_mem_used_gtt, c->used_gtt);
        rrddim_set_by_pointer(c->st_mem_usage_gtt, c->rd_mem_free_gtt, c->total_gtt - c->used_gtt);
        rrdset_done(c->st_mem_usage_gtt);
        return 0;
    }
    else {
        collector_error("Cannot read used_gtt for %s: [%s]", c->pathname, c->id.marketing_name);
        freez((void *) c->pathname_mem_used_gtt);
        freez((void *) c->pathname_mem_total_gtt);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_mem_usage_perc_gtt);
        rrdset_is_obsolete___safe_from_collector_thread(c->st_mem_usage_gtt);
        return 1;
    }
}

int do_sys_class_drm(int update_every, usec_t dt) {
    (void)dt;
    
    static DIR *drm_dir = NULL;

    int chart_prio = NETDATA_CHART_PRIO_DRM_AMDGPU;

    if(unlikely(!drm_dir)) {
        char filename[FILENAME_MAX];
        snprintfz(filename, sizeof(filename), "%s%s", netdata_configured_host_prefix, "/sys/class/drm");
        const char *drm_dir_name = inicfg_get(&netdata_config, CONFIG_SECTION_PLUGIN_PROC_DRM, "directory to monitor", filename);
        if(unlikely(NULL == (drm_dir = opendir(drm_dir_name)))){
            collector_error("Cannot read directory '%s'", drm_dir_name);
            return 1;
        }

        struct dirent *de = NULL;
        while(likely(de = readdir(drm_dir))) {
            if( de->d_type == DT_DIR && ((de->d_name[0] == '.' && de->d_name[1] == '\0') || 
                 (de->d_name[0] == '.' && de->d_name[1] == '.' && de->d_name[2] == '\0'))) continue;
            
            if(de->d_type == DT_LNK && !strncmp(de->d_name, "card", 4) && !strchr(de->d_name, '-')) {
                snprintfz(filename, sizeof(filename), "%s/%s/%s", drm_dir_name, de->d_name, "device/uevent");
                if(check_card_is_amdgpu(filename)) continue;

                /* Get static info */              

                struct card *const c = callocz(1, sizeof(struct card));
                snprintfz(filename, sizeof(filename), "%s/%s", drm_dir_name, de->d_name);
                c->pathname = strdupz(filename);

                snprintfz(filename, sizeof(filename), "%s/%s", c->pathname, "device/device");
                if(read_single_base64_or_hex_number_file(filename, &c->id.asic_id)){
                    collector_error("Cannot read asic_id from '%s'", filename);
                    card_free(c);
                    continue;
                }

                snprintfz(filename, sizeof(filename), "%s/%s", c->pathname, "device/revision");
                if(read_single_base64_or_hex_number_file(filename, &c->id.pci_rev_id)){
                    collector_error("Cannot read pci_rev_id from '%s'", filename);
                    card_free(c);
                    continue;
                }

                for(int i = 0; amdgpu_ids[i].asic_id; i++){
                    if(c->id.asic_id == amdgpu_ids[i].asic_id && c->id.pci_rev_id == amdgpu_ids[i].pci_rev_id){
                        c->id.marketing_name = strdupz(amdgpu_ids[i].marketing_name);
                        break;
                    }
                } 
                if(!c->id.marketing_name)
                    c->id.marketing_name = strdupz(amdgpu_ids[sizeof(amdgpu_ids)/sizeof(amdgpu_ids[0]) - 1].marketing_name);


                collected_number tmp_val; 
                #define set_prop_pathname_single_number_file(prop_filename, prop_pathname) do { \
                    snprintfz(filename, sizeof(filename), "%s/%s", c->pathname, prop_filename); \
                    if(!read_single_number_file(filename, (unsigned long long *) &tmp_val))     \
                        prop_pathname = strdupz(filename);                                      \
                    else                                                                        \
                        collector_info("Cannot read file '%s'", filename);                      \
                } while(0)

                #define set_prop_pathname_clock_file(prop_filename, prop_pathname, p_ff) do {   \
                    snprintfz(filename, sizeof(filename), "%s/%s", c->pathname, prop_filename); \
                    if(!read_clk_freq_file(p_ff, filename, &tmp_val))                           \
                        prop_pathname = strdupz(filename);                                      \
                    else                                                                        \
                        collector_info("Cannot read file '%s'", filename);                      \
                } while(0)

                /* Initialize GPU and VRAM utilization metrics */

                set_prop_pathname_single_number_file("device/gpu_busy_percent", c->pathname_util_gpu);
                
                if(c->pathname_util_gpu){
                    c->st_util_gpu = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_utilization", c->id.marketing_name, de->d_name)
                            , NULL
                            , "utilization"
                            , AMDGPU_CHART_TYPE ".gpu_utilization"
                            , "GPU utilization"
                            , "percentage"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdlabels_add(c->st_util_gpu->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_util_gpu = rrddim_add(c->st_util_gpu, "utilization", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                    add_do_rrd_x(c, do_rrd_util_gpu);
                }

                set_prop_pathname_single_number_file("device/mem_busy_percent", c->pathname_util_mem);

                if(c->pathname_util_mem){
                    c->st_util_mem = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_mem_utilization", c->id.marketing_name, de->d_name)
                            , NULL
                            , "utilization"
                            , AMDGPU_CHART_TYPE ".gpu_mem_utilization"
                            , "GPU memory utilization"
                            , "percentage"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdlabels_add(c->st_util_mem->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_util_mem = rrddim_add(c->st_util_mem, "utilization", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                    add_do_rrd_x(c, do_rrd_util_mem);
                }


                /* Initialize GPU and VRAM clock frequency metrics */

                set_prop_pathname_clock_file("device/pp_dpm_sclk", c->pathname_clk_gpu, &c->ff_clk_gpu);
                
                if(c->pathname_clk_gpu){
                    c->st_clk_gpu = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_clk_frequency", c->id.marketing_name, de->d_name)
                            , NULL
                            , "frequency"
                            , AMDGPU_CHART_TYPE ".gpu_clk_frequency"
                            , "GPU clock frequency"
                            , "MHz"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdlabels_add(c->st_clk_gpu->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_clk_gpu = rrddim_add(c->st_clk_gpu, "frequency", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                    add_do_rrd_x(c, do_rrd_clk_gpu);

                }

                set_prop_pathname_clock_file("device/pp_dpm_mclk", c->pathname_clk_mem, &c->ff_clk_mem);

                if(c->pathname_clk_mem){
                    c->st_clk_mem = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_mem_clk_frequency", c->id.marketing_name, de->d_name)
                            , NULL
                            , "frequency"
                            , AMDGPU_CHART_TYPE ".gpu_mem_clk_frequency"
                            , "GPU memory clock frequency"
                            , "MHz"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdlabels_add(c->st_clk_mem->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_clk_mem = rrddim_add(c->st_clk_mem, "frequency", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);

                    add_do_rrd_x(c, do_rrd_clk_mem);
                }


                /* Initialize GPU memory usage metrics */

                set_prop_pathname_single_number_file("device/mem_info_vram_used",      c->pathname_mem_used_vram);
                set_prop_pathname_single_number_file("device/mem_info_vram_total",     c->pathname_mem_total_vram);
                if(c->pathname_mem_total_vram) c->total_vram = tmp_val;
                
                if(c->pathname_mem_used_vram && c->pathname_mem_total_vram){
                    c->st_mem_usage_perc_vram = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_mem_vram_usage_perc", c->id.marketing_name, de->d_name)
                            , NULL
                            , "memory_usage"
                            , AMDGPU_CHART_TYPE ".gpu_mem_vram_usage_perc"
                            , "VRAM memory usage percentage"
                            , "percentage"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdlabels_add(c->st_mem_usage_perc_vram->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_mem_used_perc_vram = rrddim_add(c->st_mem_usage_perc_vram, "usage", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);


                    c->st_mem_usage_vram = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_mem_vram_usage", c->id.marketing_name, de->d_name)
                            , NULL
                            , "memory_usage"
                            , AMDGPU_CHART_TYPE ".gpu_mem_vram_usage"
                            , "VRAM memory usage"
                            , "bytes"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_STACKED
                    );

                    rrdlabels_add(c->st_mem_usage_vram->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_mem_free_vram = rrddim_add(c->st_mem_usage_vram, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    c->rd_mem_used_vram = rrddim_add(c->st_mem_usage_vram, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);


                    add_do_rrd_x(c, do_rrd_vram);
                }

                set_prop_pathname_single_number_file("device/mem_info_vis_vram_used",  c->pathname_mem_used_vis_vram);
                set_prop_pathname_single_number_file("device/mem_info_vis_vram_total", c->pathname_mem_total_vis_vram);
                if(c->pathname_mem_total_vis_vram) c->total_vis_vram = tmp_val;

                if(c->pathname_mem_used_vis_vram && c->pathname_mem_total_vis_vram){
                    c->st_mem_usage_perc_vis_vram = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_mem_vis_vram_usage_perc", c->id.marketing_name, de->d_name)
                            , NULL
                            , "memory_usage"
                            , AMDGPU_CHART_TYPE ".gpu_mem_vis_vram_usage_perc"
                            , "visible VRAM memory usage percentage"
                            , "percentage"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdlabels_add(c->st_mem_usage_perc_vis_vram->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_mem_used_perc_vis_vram = rrddim_add(c->st_mem_usage_perc_vis_vram, "usage", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);


                    c->st_mem_usage_vis_vram = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_mem_vis_vram_usage", c->id.marketing_name, de->d_name)
                            , NULL
                            , "memory_usage"
                            , AMDGPU_CHART_TYPE ".gpu_mem_vis_vram_usage"
                            , "visible VRAM memory usage"
                            , "bytes"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_STACKED
                    );

                    rrdlabels_add(c->st_mem_usage_vis_vram->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_mem_free_vis_vram = rrddim_add(c->st_mem_usage_vis_vram, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    c->rd_mem_used_vis_vram = rrddim_add(c->st_mem_usage_vis_vram, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);


                    add_do_rrd_x(c, do_rrd_vis_vram);
                }

                set_prop_pathname_single_number_file("device/mem_info_gtt_used",       c->pathname_mem_used_gtt);
                set_prop_pathname_single_number_file("device/mem_info_gtt_total",      c->pathname_mem_total_gtt);
                if(c->pathname_mem_total_gtt) c->total_gtt = tmp_val;

                if(c->pathname_mem_used_gtt && c->pathname_mem_total_gtt){
                    c->st_mem_usage_perc_gtt = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_mem_gtt_usage_perc", c->id.marketing_name, de->d_name)
                            , NULL
                            , "memory_usage"
                            , AMDGPU_CHART_TYPE ".gpu_mem_gtt_usage_perc"
                            , "GTT memory usage percentage"
                            , "percentage"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_LINE
                    );

                    rrdlabels_add(c->st_mem_usage_perc_gtt->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_mem_used_perc_gtt = rrddim_add(c->st_mem_usage_perc_gtt, "usage", NULL, 1, 100, RRD_ALGORITHM_ABSOLUTE);

                    c->st_mem_usage_gtt = rrdset_create_localhost(
                            AMDGPU_CHART_TYPE
                            , set_id("gpu_mem_gtt_usage", c->id.marketing_name, de->d_name)
                            , NULL
                            , "memory_usage"
                            , AMDGPU_CHART_TYPE ".gpu_mem_gtt_usage"
                            , "GTT memory usage"
                            , "bytes"
                            , PLUGIN_PROC_NAME
                            , PLUGIN_PROC_MODULE_DRM_NAME
                            , chart_prio++
                            , update_every
                            , RRDSET_TYPE_STACKED
                    );

                    rrdlabels_add(c->st_mem_usage_gtt->rrdlabels, "product_name", c->id.marketing_name, RRDLABEL_SRC_AUTO);

                    c->rd_mem_free_gtt = rrddim_add(c->st_mem_usage_gtt, "free", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    c->rd_mem_used_gtt = rrddim_add(c->st_mem_usage_gtt, "used", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);


                    add_do_rrd_x(c, do_rrd_gtt);
                }

                c->next = card_root;
                card_root = c;
            }
        }
    }


    struct card *card_cur = card_root,
                *card_next;
    while(card_cur){

        struct do_rrd_x *do_rrd_x_cur = card_cur->do_rrd_x_root,
                        *do_rrd_x_next;
        while(do_rrd_x_cur){
            if(unlikely(do_rrd_x_cur->func(card_cur))) {
                do_rrd_x_next = do_rrd_x_cur->next;
                rm_do_rrd_x(card_cur, do_rrd_x_cur);
                do_rrd_x_cur = do_rrd_x_next;
            }
            else do_rrd_x_cur = do_rrd_x_cur->next;
        }
        
        if(unlikely(!card_cur->do_rrd_x_root)){
            card_next = card_cur->next;
            card_free(card_cur);
            card_cur = card_next;
        }
        else card_cur = card_cur->next;
    }

    return card_root ? 0 : 1;
}
