// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STRING_UTF8_H
#define NETDATA_STRING_UTF8_H 1

#define IS_UTF8_BYTE(x) ((uint8_t)(x) & (uint8_t)0x80)
#define IS_UTF8_STARTBYTE(x) (IS_UTF8_BYTE(x) && ((uint8_t)(x) & (uint8_t)0x40))
#define IS_UTF8_CONTBYTE(x) (IS_UTF8_BYTE(x) && !IS_UTF8_STARTBYTE(x))

#endif /* NETDATA_STRING_UTF8_H */
