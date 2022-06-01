#ifndef PROTOCOL_COMMANDS_H
#define PROTOCOL_COMMANDS_H

#include "daemon/common.h"

#ifdef __cplusplus
extern "C" {
#endif

typedef void *command_encoder_t;

command_encoder_t command_encoder_new();
void command_encoder_delete(command_encoder_t cmd_encoder);

bool command_request_fill_gap(command_encoder_t *command_encoder, time_t after, time_t before);

#ifdef __cplusplus
}
#endif

#endif /* PROTOCOL_COMMANDS_H */
