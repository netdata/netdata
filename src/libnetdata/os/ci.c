// SPDX-License-Identifier: GPL-3.0-or-later

#include "ci.h"

#include <stdlib.h>

bool nd_is_running_under_ci(void) {
    const char *ci_vars[] = {
        "CI",                       // Generic CI flag
        "CONTINUOUS_INTEGRATION",   // Alternate generic flag
        "BUILD_NUMBER",             // Jenkins, TeamCity
        "RUN_ID",                   // AWS CodeBuild, some others
        "TRAVIS",                   // Travis CI
        "GITHUB_ACTIONS",           // GitHub Actions
        "GITHUB_TOKEN",             // GitHub Actions
        "GITLAB_CI",                // GitLab CI
        "CIRCLECI",                 // CircleCI
        "APPVEYOR",                 // AppVeyor
        "BITBUCKET_BUILD_NUMBER",   // Bitbucket Pipelines
        "SYSTEM_TEAMFOUNDATIONCOLLECTIONURI", // Azure DevOps
        "TF_BUILD",                 // Azure DevOps (alternate)
        "BAMBOO_BUILDKEY",          // Bamboo CI
        "GO_PIPELINE_NAME",         // GoCD
        "HUDSON_URL",               // Hudson CI
        "TEAMCITY_VERSION",         // TeamCity
        "CI_NAME",                  // Some environments (e.g., CodeShip)
        "CI_WORKER",                // AppVeyor (alternate)
        "CI_SERVER",                // Generic
        "HEROKU_TEST_RUN_ID",       // Heroku CI
        "BUILDKITE",                // Buildkite
        "DRONE",                    // Drone CI
        "SEMAPHORE",                // Semaphore CI
        "NETLIFY",                  // Netlify CI
        "NOW_BUILDER",              // Vercel (formerly Zeit Now)
        NULL
    };

    for (const char **env = ci_vars; *env; env++) {
        if(getenv(*env))
            return true;
    }

    return false;
}
