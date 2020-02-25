import { EndpointOptions, GraphQlQueryResponse } from "./types";
export declare class GraphqlError<T extends GraphQlQueryResponse> extends Error {
    request: EndpointOptions;
    constructor(request: EndpointOptions, response: {
        data: Required<GraphQlQueryResponse>;
    });
}
