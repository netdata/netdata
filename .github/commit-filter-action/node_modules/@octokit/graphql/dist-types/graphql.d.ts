import { request as Request } from "@octokit/request";
import { RequestParameters, GraphQlQueryResponseData } from "./types";
export declare function graphql(request: typeof Request, query: string | RequestParameters, options?: RequestParameters): Promise<GraphQlQueryResponseData>;
