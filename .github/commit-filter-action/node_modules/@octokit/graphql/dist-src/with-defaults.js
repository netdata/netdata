import { request as Request } from "@octokit/request";
import { graphql } from "./graphql";
export function withDefaults(request, newDefaults) {
    const newRequest = request.defaults(newDefaults);
    const newApi = (query, options) => {
        return graphql(newRequest, query, options);
    };
    return Object.assign(newApi, {
        defaults: withDefaults.bind(null, newRequest),
        endpoint: Request.endpoint
    });
}
