import { Octokit } from "@octokit/core";
import { OctokitResponse, RequestParameters, Route } from "./types";
export declare function iterator(octokit: Octokit, route: Route, parameters?: RequestParameters): {
    [Symbol.asyncIterator]: () => {
        next(): Promise<{
            done: boolean;
        }> | Promise<{
            value: OctokitResponse<any>;
        }>;
    };
};
