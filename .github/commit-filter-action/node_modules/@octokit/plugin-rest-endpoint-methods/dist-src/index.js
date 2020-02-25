import { Deprecation } from "deprecation";
import endpointsByScope from "./generated/endpoints";
import { VERSION } from "./version";
import { registerEndpoints } from "./register-endpoints";
/**
 * This plugin is a 1:1 copy of internal @octokit/rest plugins. The primary
 * goal is to rebuild @octokit/rest on top of @octokit/core. Once that is
 * done, we will remove the registerEndpoints methods and return the methods
 * directly as with the other plugins. At that point we will also remove the
 * legacy workarounds and deprecations.
 *
 * See the plan at
 * https://github.com/octokit/plugin-rest-endpoint-methods.js/pull/1
 */
export function restEndpointMethods(octokit) {
    // @ts-ignore
    octokit.registerEndpoints = registerEndpoints.bind(null, octokit);
    registerEndpoints(octokit, endpointsByScope);
    // Aliasing scopes for backward compatibility
    // See https://github.com/octokit/rest.js/pull/1134
    [
        ["gitdata", "git"],
        ["authorization", "oauthAuthorizations"],
        ["pullRequests", "pulls"]
    ].forEach(([deprecatedScope, scope]) => {
        Object.defineProperty(octokit, deprecatedScope, {
            get() {
                octokit.log.warn(
                // @ts-ignore
                new Deprecation(`[@octokit/plugin-rest-endpoint-methods] "octokit.${deprecatedScope}.*" methods are deprecated, use "octokit.${scope}.*" instead`));
                // @ts-ignore
                return octokit[scope];
            }
        });
    });
    return {};
}
restEndpointMethods.VERSION = VERSION;
