import { Octokit } from "@octokit/core";
import { Api } from "./types";
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
export declare function restEndpointMethods(octokit: Octokit): Api;
export declare namespace restEndpointMethods {
    var VERSION: string;
}
