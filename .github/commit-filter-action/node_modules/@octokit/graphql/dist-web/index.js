import { request } from '@octokit/request';
import { getUserAgent } from 'universal-user-agent';

const VERSION = "4.3.1";

class GraphqlError extends Error {
    constructor(request, response) {
        const message = response.data.errors[0].message;
        super(message);
        Object.assign(this, response.data);
        this.name = "GraphqlError";
        this.request = request;
        // Maintains proper stack trace (only available on V8)
        /* istanbul ignore next */
        if (Error.captureStackTrace) {
            Error.captureStackTrace(this, this.constructor);
        }
    }
}

const NON_VARIABLE_OPTIONS = [
    "method",
    "baseUrl",
    "url",
    "headers",
    "request",
    "query"
];
function graphql(request, query, options) {
    options =
        typeof query === "string"
            ? (options = Object.assign({ query }, options))
            : (options = query);
    const requestOptions = Object.keys(options).reduce((result, key) => {
        if (NON_VARIABLE_OPTIONS.includes(key)) {
            result[key] = options[key];
            return result;
        }
        if (!result.variables) {
            result.variables = {};
        }
        result.variables[key] = options[key];
        return result;
    }, {});
    return request(requestOptions).then(response => {
        if (response.data.errors) {
            throw new GraphqlError(requestOptions, {
                data: response.data
            });
        }
        return response.data.data;
    });
}

function withDefaults(request$1, newDefaults) {
    const newRequest = request$1.defaults(newDefaults);
    const newApi = (query, options) => {
        return graphql(newRequest, query, options);
    };
    return Object.assign(newApi, {
        defaults: withDefaults.bind(null, newRequest),
        endpoint: request.endpoint
    });
}

const graphql$1 = withDefaults(request, {
    headers: {
        "user-agent": `octokit-graphql.js/${VERSION} ${getUserAgent()}`
    },
    method: "POST",
    url: "/graphql"
});
function withCustomRequest(customRequest) {
    return withDefaults(customRequest, {
        method: "POST",
        url: "/graphql"
    });
}

export { graphql$1 as graphql, withCustomRequest };
//# sourceMappingURL=index.js.map
