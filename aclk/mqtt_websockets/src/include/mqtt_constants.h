// Copyright: SPDX-License-Identifier:  GPL-3.0-only

#ifndef MQTT_CONSTANTS_H
#define MQTT_CONSTANTS_H

#define MQTT_MAX_QOS 0x02

#define MQTT_VERSION_5_0     0x5

/* [MQTT-1.5.5] most significant bit
   of MQTT Variable Byte Integer signifies
   there are more bytes following */
#define MQTT_VBI_CONTINUATION_FLAG 0x80
#define MQTT_VBI_DATA_MASK         0x7F
#define MQTT_VBI_MAXBYTES          4

/* MQTT control packet types as defined in
   2.1.2 MQTT Control Packet type */
#define MQTT_CPT_CONNECT     0x1
#define MQTT_CPT_CONNACK     0x2
#define MQTT_CPT_PUBLISH     0x3
#define MQTT_CPT_PUBACK      0x4
#define MQTT_CPT_PUBREC      0x5
#define MQTT_CPT_PUBREL      0x6
#define MQTT_CPT_PUBCOMP     0x7
#define MQTT_CPT_SUBSCRIBE   0x8
#define MQTT_CPT_SUBACK      0x9
#define MQTT_CPT_UNSUBSCRIBE 0xA
#define MQTT_CPT_UNSUBACK    0xB
#define MQTT_CPT_PINGREQ     0xC
#define MQTT_CPT_PINGRESP    0xD
#define MQTT_CPT_DISCONNECT  0xE
#define MQTT_CPT_AUTH        0xF

// MQTT CONNECT FLAGS (spec:3.1.2.3)
#define MQTT_CONNECT_FLAG_USERNAME    0x80
#define MQTT_CONNECT_FLAG_PASSWORD    0x40
#define MQTT_CONNECT_FLAG_LWT_RETAIN  0x20
#define MQTT_CONNECT_FLAG_LWT         0x04
#define MQTT_CONNECT_FLAG_CLEAN_START 0x02

#define MQTT_CONNECT_FLAG_QOS_MASK    0x18
#define MQTT_CONNECT_FLAG_QOS_BITSHIFT 3

#define MQTT_MAX_CLIENT_ID 23 /* [MQTT-3.1.3-5] */

// MQTT Property identifiers [MQTT-2.2.2.2]
#define MQTT_PROP_PAYLOAD_FMT_INDICATOR        0x01
#define MQTT_PROP_PAYLOAD_FMT_INDICATOR_NAME   "Payload Format Indicator"
#define MQTT_PROP_MSG_EXPIRY_INTERVAL          0x02
#define MQTT_PROP_MSG_EXPIRY_INTERVAL_NAME     "Message Expiry Interval"
#define MQTT_PROP_CONTENT_TYPE                 0x03
#define MQTT_PROP_CONTENT_TYPE_NAME            "Content Type"
#define MQTT_PROP_RESPONSE_TOPIC               0x08
#define MQTT_PROP_RESPONSE_TOPIC_NAME          "Response Topic"
#define MQTT_PROP_CORRELATION_DATA             0x09
#define MQTT_PROP_CORRELATION_DATA_NAME        "Correlation Data"
#define MQTT_PROP_SUB_IDENTIFIER               0x0B
#define MQTT_PROP_SUB_IDENTIFIER_NAME          "Subscription Identifier"
#define MQTT_PROP_SESSION_EXPIRY_INTERVAL      0x11
#define MQTT_PROP_SESSION_EXPIRY_INTERVAL_NAME "Session Expiry Interval"
#define MQTT_PROP_ASSIGNED_CLIENT_ID           0x12
#define MQTT_PROP_ASSIGNED_CLIENT_ID_NAME      "Assigned Client Identifier"
#define MQTT_PROP_SERVER_KEEP_ALIVE            0x13
#define MQTT_PROP_SERVER_KEEP_ALIVE_NAME       "Server Keep Alive"
#define MQTT_PROP_AUTH_METHOD                  0x15
#define MQTT_PROP_AUTH_METHOD_NAME             "Authentication Method"
#define MQTT_PROP_AUTH_DATA                    0x16
#define MQTT_PROP_AUTH_DATA_NAME               "Authentication Data"
#define MQTT_PROP_REQ_PROBLEM_INFO             0x17
#define MQTT_PROP_REQ_PROBLEM_INFO_NAME        "Request Problem Information"
#define MQTT_PROP_WILL_DELAY_INTERVAL          0x18
#define MQTT_PROP_WIIL_DELAY_INTERVAL_NAME     "Will Delay Interval"
#define MQTT_PROP_REQ_RESP_INFORMATION         0x19
#define MQTT_PROP_REQ_RESP_INFORMATION_NAME    "Request Response Information"
#define MQTT_PROP_RESP_INFORMATION             0x1A
#define MQTT_PROP_RESP_INFORMATION_NAME        "Response Information"
#define MQTT_PROP_SERVER_REF                   0x1C
#define MQTT_PROP_SERVER_REF_NAME              "Server Reference"
#define MQTT_PROP_REASON_STR                   0x1F
#define MQTT_PROP_REASON_STR_NAME              "Reason String"
#define MQTT_PROP_RECEIVE_MAX                  0x21
#define MQTT_PROP_RECEIVE_MAX_NAME             "Receive Maximum"
#define MQTT_PROP_TOPIC_ALIAS_MAX              0x22
#define MQTT_PROP_TOPIC_ALIAS_MAX_NAME         "Topic Alias Maximum"
#define MQTT_PROP_TOPIC_ALIAS                  0x23
#define MQTT_PROP_TOPIC_ALIAS_NAME             "Topic Alias"
#define MQTT_PROP_MAX_QOS                      0x24
#define MQTT_PROP_MAX_QOS_NAME                 "Maximum QoS"
#define MQTT_PROP_RETAIN_AVAIL                 0x25
#define MQTT_PROP_RETAIN_AVAIL_NAME            "Retain Available"
#define MQTT_PROP_USR                          0x26
#define MQTT_PROP_USR_NAME                     "User Property"
#define MQTT_PROP_MAX_PKT_SIZE                 0x27
#define MQTT_PROP_MAX_PKT_SIZE_NAME            "Maximum Packet Size"
#define MQTT_PROP_WILDCARD_SUB_AVAIL           0x28
#define MQTT_PROP_WILDCARD_SUB_AVAIL_NAME      "Wildcard Subscription Available"
#define MQTT_PROP_SUB_ID_AVAIL                 0x29
#define MQTT_PROP_SUB_ID_AVAIL_NAME            "Subscription Identifier Available"
#define MQTT_PROP_SHARED_SUB_AVAIL             0x2A
#define MQTT_PROP_SHARED_SUB_AVAIL_NAME        "Shared Subscription Available"

#endif /* MQTT_CONSTANTS_H */
