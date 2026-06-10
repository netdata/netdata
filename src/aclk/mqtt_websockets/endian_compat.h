// SPDX-License-Identifier: GPL-3.0-only

#ifndef MQTT_WSS_ENDIAN_COMPAT_H
#define MQTT_WSS_ENDIAN_COMPAT_H

#ifdef __APPLE__
    #include <libkern/OSByteOrder.h>

    #define htobe16(x) OSSwapHostToBigInt16(x)
    #define htole16(x) OSSwapHostToLittleInt16(x)
    #define be16toh(x) OSSwapBigToHostInt16(x)
    #define le16toh(x) OSSwapLittleToHostInt16(x)

    #define htobe32(x) OSSwapHostToBigInt32(x)
    #define htole32(x) OSSwapHostToLittleInt32(x)
    #define be32toh(x) OSSwapBigToHostInt32(x)
    #define le32toh(x) OSSwapLittleToHostInt32(x)

    #define htobe64(x) OSSwapHostToBigInt64(x)
    #define htole64(x) OSSwapHostToLittleInt64(x)
    #define be64toh(x) OSSwapBigToHostInt64(x)
    #define le64toh(x) OSSwapLittleToHostInt64(x)
#else
#ifdef __FreeBSD__
    #include <sys/endian.h>
#elif defined(_WIN32)
    // Windows/UCRT64 has no <endian.h>; x86_64 is always little-endian.
    #ifndef htobe16
    #define htobe16(x) __builtin_bswap16(x)
    #define htole16(x) (x)
    #define be16toh(x) __builtin_bswap16(x)
    #define le16toh(x) (x)
    #define htobe32(x) __builtin_bswap32(x)
    #define htobe64(x) __builtin_bswap64(x)
    #define be32toh(x) __builtin_bswap32(x)
    #define be64toh(x) __builtin_bswap64(x)
    #endif
    #ifndef htole32
    #define htole32(x) (x)
    #define le32toh(x) (x)
    #define htole64(x) (x)
    #define le64toh(x) (x)
    #endif
#else
    #include <endian.h>
#endif
#endif

#endif /* MQTT_WSS_ENDIAN_COMPAT_H */
