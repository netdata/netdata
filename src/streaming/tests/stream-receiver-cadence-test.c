// SPDX-License-Identifier: GPL-3.0-or-later

#include "streaming/stream-receiver-cadence.h"
#include "streaming/stream-receiver-socket.h"

int main(void) {
    int errors = stream_receiver_cadence_unittest();
    errors += stream_receiver_socket_unittest();
    return errors;
}
