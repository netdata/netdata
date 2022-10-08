#include <gtest/gtest.h>
#include "daemon/common.h"

TEST(buffer, sprintf) {
    const char *Fmt = "string1: %s\nstring2: %s\nstring3: %s\nstring4: %s";

    char Dummy[2048 + 1];
    for(int Idx = 0; Idx != 2048; Idx++)
        Dummy[Idx] = ((Idx % 24) + 'a');
    Dummy[2048] = '\0';

    char Expected[9000 + 1];
    snprintfz(Expected, 9000, Fmt, Dummy, Dummy, Dummy, Dummy);

    BUFFER *WB = buffer_create(1);
    buffer_sprintf(WB, Fmt, Dummy, Dummy, Dummy, Dummy);

    const char *Output = buffer_tostring(WB);
    EXPECT_STREQ(Expected, Output);

    buffer_free(WB);
}
