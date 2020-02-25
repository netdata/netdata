import { request as Request } from "@octokit/request";
import { graphql as ApiInterface, RequestParameters } from "./types";
export declare function withDefaults(request: typeof Request, newDefaults: RequestParameters): ApiInterface;
