// Copyright: SPDX-License-Identifier:  GPL-3.0-only

#include <stdlib.h>
#include <stdarg.h>
#include <string.h>
#include <stdio.h>

#include "mqtt_wss_log.h"
#include "common_internal.h"

struct mqtt_wss_log_ctx {
    mqtt_wss_log_callback_t extern_log_fnc;
    char *ctx_prefix;
    char *buffer;
    char *buffer_w_ptr;
    size_t buffer_bytes_avail;
};

#define LOG_BUFFER_SIZE 1024 * 4
#define LOG_CTX_PREFIX_SEV_STR "  : "
#define LOG_CTX_PREFIX_LIMIT 15
#define LOG_CTX_PREFIX_LIMIT_STR (LOG_CTX_PREFIX_LIMIT - (2 + strlen(LOG_CTX_PREFIX_SEV_STR)))  // with [] characters and affixed ' ' it is total 15 chars
#if (LOG_CTX_PREFIX_LIMIT * 10) > LOG_BUFFER_SIZE
#error "LOG_BUFFER_SIZE too small"
#endif
mqtt_wss_log_ctx_t mqtt_wss_log_ctx_create(const char *ctx_prefix, mqtt_wss_log_callback_t log_callback)
{
    mqtt_wss_log_ctx_t ctx = mw_calloc(1, sizeof(struct mqtt_wss_log_ctx));
    if(!ctx)
        return NULL;

    if(log_callback) {
        ctx->extern_log_fnc = log_callback;
        ctx->buffer = mw_calloc(1, LOG_BUFFER_SIZE);
        if(!ctx->buffer)
            goto cleanup;

        ctx->buffer_w_ptr = ctx->buffer;
        if(ctx_prefix) {
            *(ctx->buffer_w_ptr++) = '[';
            strncpy(ctx->buffer_w_ptr, ctx_prefix, LOG_CTX_PREFIX_LIMIT_STR);
            ctx->buffer_w_ptr += strnlen(ctx_prefix, LOG_CTX_PREFIX_LIMIT_STR);
            *(ctx->buffer_w_ptr++) = ']';
        }
        strcpy(ctx->buffer_w_ptr, LOG_CTX_PREFIX_SEV_STR);
        ctx->buffer_w_ptr += strlen(LOG_CTX_PREFIX_SEV_STR);
        // no term '\0' -> calloc is used

        ctx->buffer_bytes_avail = LOG_BUFFER_SIZE - strlen(ctx->buffer);

        return ctx;
    }

    if(ctx_prefix) {
        ctx->ctx_prefix = strndup(ctx_prefix, LOG_CTX_PREFIX_LIMIT_STR);
        if(!ctx->ctx_prefix)
            goto cleanup;
    }

    return ctx;

cleanup:
    mw_free(ctx);
    return NULL;
}

void mqtt_wss_log_ctx_destroy(mqtt_wss_log_ctx_t ctx)
{
    mw_free(ctx->ctx_prefix);
    mw_free(ctx->buffer);
    mw_free(ctx);
}

static inline char severity_to_c(int severity)
{
    switch (severity) {
        case MQTT_WSS_LOG_FATAL:
            return 'F';
        case MQTT_WSS_LOG_ERROR:
            return 'E';
        case MQTT_WSS_LOG_WARN:
            return 'W';
        case MQTT_WSS_LOG_INFO:
            return 'I';
        case MQTT_WSS_LOG_DEBUG:
            return 'D';
        default:
            return '?';
    }
}

void mws_log(int severity, mqtt_wss_log_ctx_t ctx, const char *fmt, va_list args)
{
    size_t size;

    if(ctx->extern_log_fnc) {
        size = vsnprintf(ctx->buffer_w_ptr, ctx->buffer_bytes_avail, fmt, args);
        *(ctx->buffer_w_ptr - 3) = severity_to_c(severity);

        ctx->extern_log_fnc(severity, ctx->buffer);

        if(size >= ctx->buffer_bytes_avail)
            mws_error(ctx, "Last message of this type was truncated! Consider what you log or increase LOG_BUFFER_SIZE if really needed.");

        return;
    }

    if(ctx->ctx_prefix)
        printf("[%s] ", ctx->ctx_prefix);

    printf("%c: ", severity_to_c(severity));

    vprintf(fmt, args);
    putchar('\n');
}

#define DEFINE_MWS_SEV_FNC(severity_fncname, severity) \
void mws_ ## severity_fncname(mqtt_wss_log_ctx_t ctx, const char *fmt, ...) \
{ \
    va_list args; \
    va_start(args, fmt); \
    mws_log(severity, ctx, fmt, args); \
    va_end(args); \
}

DEFINE_MWS_SEV_FNC(fatal, MQTT_WSS_LOG_FATAL)
DEFINE_MWS_SEV_FNC(error, MQTT_WSS_LOG_ERROR)
DEFINE_MWS_SEV_FNC(warn,  MQTT_WSS_LOG_WARN )
DEFINE_MWS_SEV_FNC(info,  MQTT_WSS_LOG_INFO )
DEFINE_MWS_SEV_FNC(debug, MQTT_WSS_LOG_DEBUG)
