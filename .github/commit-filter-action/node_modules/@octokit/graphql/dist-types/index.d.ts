import { request } from "@octokit/request";
export declare const graphql: import("./types").graphql;
export declare function withCustomRequest(customRequest: typeof request): import("./types").graphql;
