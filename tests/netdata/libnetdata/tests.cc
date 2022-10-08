#include <gtest/gtest.h>
#include "daemon/common.h"

TEST(libnetdata, strdupz_path_subpath) {
    struct PathParts {
        const char *Path1;
        const char *Path2;
    };
    std::vector<std::pair<PathParts, const char *>> Values {
        { PathParts{"", ""}, "." },
        { PathParts{"/", ""}, "/" },
        { PathParts{"/etc/netdata", ""}, "/etc/netdata" },
        { PathParts{"/etc/netdata///", ""}, "/etc/netdata" },
        { PathParts{"/etc/netdata///", "health.d"}, "/etc/netdata/health.d" },
        { PathParts{"/etc/netdata///", "///health.d"}, "/etc/netdata/health.d" },
        { PathParts{"/etc/netdata", "///health.d"}, "/etc/netdata/health.d" },
        { PathParts{"", "///health.d"}, "./health.d" },
        { PathParts{"/", "///health.d"}, "/health.d" },
    };

    for (const auto &P : Values) {
        struct PathParts PP = P.first;
        const char *Expected = P.second;

        char *Res = strdupz_path_subpath(PP.Path1, PP.Path2);
        EXPECT_STREQ(Res, Expected);
        freez(Res);
    }
}
