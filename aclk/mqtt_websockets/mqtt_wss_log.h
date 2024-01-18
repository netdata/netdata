// Copyright: SPDX-License-Identifier:  GPL-3.0-only

#ifndef MQTT_WSS_LOG_H
#define MQTT_WSS_LOG_H

typedef enum mqtt_wss_log_type {
    MQTT_WSS_LOG_DEBUG = 0x01,
    MQTT_WSS_LOG_INFO  = 0x02,
    MQTT_WSS_LOG_WARN  = 0x03,
    MQTT_WSS_LOG_ERROR = 0x81,
    MQTT_WSS_LOG_FATAL = 0x88
} mqtt_wss_log_type_t;

typedef void (*mqtt_wss_log_callback_t)(mqtt_wss_log_type_t, const char*);

typedef struct mqtt_wss_log_ctx *mqtt_wss_log_ctx_t;

/** Creates logging context with optional prefix and optional callback
 *   @param ctx_prefix String to be prefixed to every log message.
 *   This is useful if multiple clients are instantiated to be able to
 *   know which one this message belongs to. Can be `NULL` for no prefix.
 *   @param log_callback Callback to be called instead of logging to
 *   `STDOUT` or `STDERR` (if debug enabled otherwise silent). Callback has to be
 *   pointer to function of `void function(mqtt_wss_log_type_t, const char*)` type.
 *   If `NULL` default will be used (silent or STDERR/STDOUT).
 *   @return mqtt_wss_log_ctx_t or `NULL` on error */
mqtt_wss_log_ctx_t mqtt_wss_log_ctx_create(const char *ctx_prefix, mqtt_wss_log_callback_t log_callback);

/** Destroys logging context and cleans up the memory
 *  @param ctx Context to destroy */
void mqtt_wss_log_ctx_destroy(mqtt_wss_log_ctx_t ctx);

void mws_fatal(mqtt_wss_log_ctx_t ctx, const char *fmt, ...);
void mws_error(mqtt_wss_log_ctx_t ctx, const char *fmt, ...);
void mws_warn (mqtt_wss_log_ctx_t ctx, const char *fmt, ...);
void mws_info (mqtt_wss_log_ctx_t ctx, const char *fmt, ...);
void mws_debug(mqtt_wss_log_ctx_t ctx, const char *fmt, ...);

#endif /* MQTT_WSS_LOG_H */
