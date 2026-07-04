// SPDX-License-Identifier: GPL-3.0-or-later

#include "macos_smc.h"

#include <math.h>
#include <string.h>

struct macos_smc_key_data_ver {
    uint8_t major;
    uint8_t minor;
    uint8_t build;
    uint8_t reserved;
    uint16_t release;
};

struct macos_smc_p_limit_data {
    uint16_t version;
    uint16_t length;
    uint32_t cpu_p_limit;
    uint32_t gpu_p_limit;
    uint32_t mem_p_limit;
};

struct macos_smc_key_info {
    uint32_t data_size;
    uint32_t data_type;
    uint8_t data_attributes;
};

struct macos_smc_key_data {
    uint32_t key;
    struct macos_smc_key_data_ver vers;
    struct macos_smc_p_limit_data p_limit_data;
    struct macos_smc_key_info key_info;
    uint8_t result;
    uint8_t status;
    uint8_t data8;
    uint32_t data32;
    uint8_t bytes[MACOS_SMC_MAX_VALUE_SIZE];
};

static uint16_t macos_smc_read_be16(const uint8_t *p)
{
    return ((uint16_t)p[0] << 8) | (uint16_t)p[1];
}

static uint32_t macos_smc_read_be32(const uint8_t *p)
{
    return ((uint32_t)p[0] << 24) | ((uint32_t)p[1] << 16) | ((uint32_t)p[2] << 8) | (uint32_t)p[3];
}

static uint32_t macos_smc_read_le32(const uint8_t *p)
{
    return (uint32_t)p[0] | ((uint32_t)p[1] << 8) | ((uint32_t)p[2] << 16) | ((uint32_t)p[3] << 24);
}

static uint64_t macos_smc_read_le64(const uint8_t *p)
{
    uint64_t value = 0;
    for (size_t i = 0; i < 8; i++)
        value |= (uint64_t)p[i] << (i * 8);

    return value;
}

static uint32_t macos_smc_key_from_cstr(const char key[MACOS_SMC_KEY_LEN + 1])
{
    return ((uint32_t)(uint8_t)key[0] << 24) | ((uint32_t)(uint8_t)key[1] << 16) |
           ((uint32_t)(uint8_t)key[2] << 8) | (uint32_t)(uint8_t)key[3];
}

static void macos_smc_key_to_cstr(uint32_t key, char dst[MACOS_SMC_KEY_LEN + 1])
{
    dst[0] = (char)((key >> 24) & 0xff);
    dst[1] = (char)((key >> 16) & 0xff);
    dst[2] = (char)((key >> 8) & 0xff);
    dst[3] = (char)(key & 0xff);
    dst[4] = '\0';
}

static bool macos_smc_call(
    io_connect_t connection,
    const struct macos_smc_key_data *input,
    struct macos_smc_key_data *output)
{
    size_t output_size = sizeof(*output);
    memset(output, 0, sizeof(*output));

    IOReturn kr = IOConnectCallStructMethod(connection, 2, input, sizeof(*input), output, &output_size);
    if (kr != kIOReturnSuccess)
        return false;

    return output->result == 0;
}

static bool macos_smc_read_key_info(io_connect_t connection, uint32_t key, struct macos_smc_key_info *info)
{
    struct macos_smc_key_data input = {
        .key = key,
        .data8 = 9,
    };
    struct macos_smc_key_data output;

    if (!macos_smc_call(connection, &input, &output))
        return false;

    *info = output.key_info;
    return true;
}

static bool macos_smc_read_value(
    io_connect_t connection,
    uint32_t key,
    const struct macos_smc_key_info *info,
    struct macos_smc_key_data *output)
{
    struct macos_smc_key_data input = {
        .key = key,
        .key_info = *info,
        .data8 = 5,
    };

    return macos_smc_call(connection, &input, output);
}

bool macos_smc_open(io_connect_t *connection)
{
    if (!connection)
        return false;

    *connection = IO_OBJECT_NULL;

    io_iterator_t iter = IO_OBJECT_NULL;
    IOReturn kr = IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("AppleSMC"), &iter);
    if (kr != kIOReturnSuccess || iter == IO_OBJECT_NULL)
        return false;

    bool opened = false;
    io_service_t service;
    while ((service = IOIteratorNext(iter)) != IO_OBJECT_NULL) {
        io_name_t name;
        if (IORegistryEntryGetName(service, name) == kIOReturnSuccess && !strcmp(name, "AppleSMCKeysEndpoint")) {
            kr = IOServiceOpen(service, mach_task_self(), 0, connection);
            opened = kr == kIOReturnSuccess;
        }
        IOObjectRelease(service);

        if (opened)
            break;
    }

    IOObjectRelease(iter);
    return opened;
}

void macos_smc_close(io_connect_t *connection)
{
    if (!connection || *connection == IO_OBJECT_NULL)
        return;

    IOServiceClose(*connection);
    *connection = IO_OBJECT_NULL;
}

bool macos_smc_key_count(io_connect_t connection, uint32_t *count)
{
    if (!count)
        return false;

    struct macos_smc_value value;
    if (!macos_smc_read_key(connection, "#KEY", &value) || value.size < 4)
        return false;

    *count = macos_smc_read_be32(value.bytes);
    return true;
}

bool macos_smc_key_by_index(io_connect_t connection, uint32_t index, char key[MACOS_SMC_KEY_LEN + 1])
{
    if (!key)
        return false;

    struct macos_smc_key_data input = {
        .data8 = 8,
        .data32 = index,
    };
    struct macos_smc_key_data output;

    if (!macos_smc_call(connection, &input, &output))
        return false;

    macos_smc_key_to_cstr(output.key, key);
    return true;
}

bool macos_smc_read_key(io_connect_t connection, const char key[MACOS_SMC_KEY_LEN + 1], struct macos_smc_value *value)
{
    if (!key || !value)
        return false;

    uint32_t smc_key = macos_smc_key_from_cstr(key);
    struct macos_smc_key_info info;
    if (!macos_smc_read_key_info(connection, smc_key, &info))
        return false;

    if (info.data_size > MACOS_SMC_MAX_VALUE_SIZE)
        return false;

    struct macos_smc_key_data output;
    if (!macos_smc_read_value(connection, smc_key, &info, &output))
        return false;

    memset(value, 0, sizeof(*value));
    snprintfz(value->key, sizeof(value->key), "%s", key);
    macos_smc_key_to_cstr(info.data_type, value->type);
    value->size = info.data_size;
    memcpy(value->bytes, output.bytes, value->size);
    return true;
}

static int macos_smc_hex_value(char c)
{
    if (c >= '0' && c <= '9')
        return c - '0';
    if (c >= 'a' && c <= 'f')
        return c - 'a' + 10;
    if (c >= 'A' && c <= 'F')
        return c - 'A' + 10;

    return -1;
}

static bool macos_smc_decode_fixed_16(const struct macos_smc_value *value, bool is_signed, NETDATA_DOUBLE *decoded)
{
    if (value->size < 2)
        return false;

    int fractional_bits = macos_smc_hex_value(value->type[3]);
    if (fractional_bits < 0 || fractional_bits > 15)
        return false;

    NETDATA_DOUBLE divisor = (NETDATA_DOUBLE)(1U << fractional_bits);
    if (is_signed) {
        int16_t raw = (int16_t)macos_smc_read_be16(value->bytes);
        *decoded = (NETDATA_DOUBLE)raw / divisor;
    } else {
        uint16_t raw = macos_smc_read_be16(value->bytes);
        *decoded = (NETDATA_DOUBLE)raw / divisor;
    }

    return isfinite(*decoded);
}

bool macos_smc_decode_numeric(const struct macos_smc_value *value, NETDATA_DOUBLE *decoded)
{
    if (!value || !decoded)
        return false;

    if (!strcmp(value->type, "flt ") && value->size >= 4) {
        uint32_t raw = macos_smc_read_le32(value->bytes);
        float f;
        memcpy(&f, &raw, sizeof(f));
        *decoded = (NETDATA_DOUBLE)f;
        return isfinite(*decoded);
    }

    if (!strcmp(value->type, "ioft") && value->size >= 8) {
        *decoded = (NETDATA_DOUBLE)macos_smc_read_le64(value->bytes) / 65536.0;
        return isfinite(*decoded);
    }

    if (value->type[0] == 'f' && value->type[1] == 'p')
        return macos_smc_decode_fixed_16(value, false, decoded);

    if (value->type[0] == 's' && value->type[1] == 'p')
        return macos_smc_decode_fixed_16(value, true, decoded);

    if (!strcmp(value->type, "ui8 ") && value->size >= 1) {
        *decoded = value->bytes[0];
        return true;
    }

    if (!strcmp(value->type, "ui16") && value->size >= 2) {
        *decoded = macos_smc_read_be16(value->bytes);
        return true;
    }

    if (!strcmp(value->type, "ui32") && value->size >= 4) {
        *decoded = macos_smc_read_be32(value->bytes);
        return true;
    }

    if (!strcmp(value->type, "si8 ") && value->size >= 1) {
        *decoded = (int8_t)value->bytes[0];
        return true;
    }

    if (!strcmp(value->type, "si16") && value->size >= 2) {
        *decoded = (int16_t)macos_smc_read_be16(value->bytes);
        return true;
    }

    if (!strcmp(value->type, "si32") && value->size >= 4) {
        *decoded = (int32_t)macos_smc_read_be32(value->bytes);
        return true;
    }

    return false;
}

bool macos_smc_decode_temperature(const struct macos_smc_value *value, NETDATA_DOUBLE *decoded)
{
    if (!value || !decoded)
        return false;

    NETDATA_DOUBLE temperature;

    if (!strcmp(value->key, "Ta0P") && value->size >= 2) {
        struct macos_smc_value corrected = *value;
        snprintfz(corrected.type, sizeof(corrected.type), "sp78");
        if (!macos_smc_decode_numeric(&corrected, &temperature))
            return false;
    } else if (!macos_smc_decode_numeric(value, &temperature))
        return false;

    if (!isfinite(temperature) || temperature <= 0.0 || temperature > 150.0)
        return false;

    *decoded = temperature;
    return true;
}
