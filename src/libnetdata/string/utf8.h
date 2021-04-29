// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STRING_UTF8_H
#define NETDATA_STRING_UTF8_H 1

#define IS_UTF8_BYTE(x) (x & 0x80)
#define IS_UTF8_STARTBYTE(x) (IS_UTF8_BYTE(x)&&(x & 0x40))

#endif /* NETDATA_STRING_UTF8_H */
