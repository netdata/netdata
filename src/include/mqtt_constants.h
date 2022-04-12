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

#endif /* MQTT_CONSTANTS_H */
