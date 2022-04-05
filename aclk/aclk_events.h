// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ACLK_EVENTS_H
#define ACLK_EVENTS_H

typedef uint64_t aclk_event_log_t;

// for more visual documentation look at aclk/docs/aclk_event_code_description.pdf
// event code looks like this (one char = one Byte, spaces for clarity only)
// GG RB UU EE
// G - Group
// R - Reserved for future use
// B - Bitfield - see per bit details below
// U - User Group - e.g. mqtt_websockets library is already grouping its codes
// E - Event Code - same event code describes different event if Group or User Group is different

// B Bitfield is a Byte defined as follows (one char one bit, spaces for clarity only)
// RRRR RRRE
// R - Reserved for future use
// E - bit signifying this event is an error

// Example of future use of User Group
// Netdata agent will have Group MQTT_WEBSOCKETS already for all events comming from this library
// library itself can however group events on its own will e.g. related to http client, related to
// websockets connection, related to MQTT etc.

#define ACLK_EVT_ERROR_BIT ((aclk_event_log_t) 1 << 32)

#define GROUP_SHIFT 48

#define aclk_evt_is_error(CODE) (( (aclk_event_log_t)CODE & ACLK_EVT_ERROR_BIT ) ? 1 : 0)

#define ACLK_EVT_DEFINE(FAC, IDX, ERROR) ( ((aclk_event_log_t) FAC << GROUP_SHIFT) | IDX | (ERROR ? ACLK_EVT_ERROR_BIT : 0) )

// In case some event ID is obsoleted do not reuse it
// comment it out if you want but dont reuse
// that would cause confusion on cloud side

// details and errors related to /api/v1/env
#define ACLK_EVT_GRP_OTP_ENV 1
#define ACLK_EVT_OTP_ENV_BEGIN ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_ENV, 1, 0)
#define ACLK_EVT_OTP_ENV_DONE  ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_ENV, 2, 0)
#define ACLK_EVT_NEW_PROTO_SWITCH ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_ENV, 3, 0)
#define ACLK_EVT_ENV_NEGOTIATION_FAILURE ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_ENV, 4, 1)
#define ACLK_EVT_ENV_URL_ERROR ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_ENV, 5, 1)
#define ACLK_EVT_ENV_NO_LWT_TOPIC ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_ENV, 6, 1)
#define ACLK_EVT_ENV_NO_USABLE_TRANSPORT ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_ENV, 7, 1)
#define ACLK_EVT_ENV_TARGET_URL_ERR ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_ENV, 8, 1)

// details and errors related to authentication, getting challenge over HTTP
#define ACLK_EVT_GRP_OTP_CHALLENGE 2
#define ACLK_EVT_CHALLENGE_BEGIN ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_CHALLENGE, 1, 0)
#define ACLK_EVT_CHALLENGE_RCVD ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_CHALLENGE, 2, 0)
#define ACLK_EVT_CHALLENGE_PARSE_ERR ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_CHALLENGE, 3, 1)
#define ACLK_EVT_CHALLENGE_GET_ERR ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_CHALLENGE, 4, 1)
#define ACLK_EVT_CHALLENGE_NOT_200 ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_CHALLENGE, 5, 1)

// details and errors related to authentication, replying to challenge and getting password over HTTP
#define ACLK_EVT_GRP_OTP_PASSWORD 3
#define ACLK_EVT_PASSWD_BEGIN ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_PASSWORD, 1, 0)
#define ACLK_EVT_PASSWD_DONE ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_PASSWORD, 2, 0)
#define ACLK_EVT_PASSWD_POST_ERR ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_PASSWORD, 3, 1)
#define ACLK_EVT_PASSWD_NOT_201 ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_PASSWORD, 4, 1)
#define ACLK_EVT_PASSWD_RESPONSE_PARSE_ERROR ACLK_EVT_DEFINE(ACLK_EVT_GRP_OTP_PASSWORD, 5, 1)

// high importance events on high level
// like CONNECTED/DISCONNECTED
#define ACLK_EVT_GRP_HIGH_LEVEL 4
#define ACLK_EVT_CONN_EST ACLK_EVT_DEFINE(ACLK_EVT_GRP_HIGH_LEVEL, 1, 0)
#define ACLK_EVT_CONN_DROP ACLK_EVT_DEFINE(ACLK_EVT_GRP_HIGH_LEVEL, 2, 1)
#define ACLK_EVT_CONN_GRACEFUL_DISCONNECT ACLK_EVT_DEFINE(ACLK_EVT_GRP_HIGH_LEVEL, 3, 0)
#define ACLK_EVT_MQTT_PUBACK_LIMIT ACLK_EVT_DEFINE(ACLK_EVT_GRP_HIGH_LEVEL, 4, 0)

#endif /* ACLK_EVENTS_H */
