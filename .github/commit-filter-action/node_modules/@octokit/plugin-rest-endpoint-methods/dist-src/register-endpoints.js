import { Deprecation } from "deprecation";
export function registerEndpoints(octokit, routes) {
    Object.keys(routes).forEach(namespaceName => {
        if (!octokit[namespaceName]) {
            octokit[namespaceName] = {};
        }
        Object.keys(routes[namespaceName]).forEach(apiName => {
            const apiOptions = routes[namespaceName][apiName];
            const endpointDefaults = ["method", "url", "headers"].reduce((map, key) => {
                if (typeof apiOptions[key] !== "undefined") {
                    map[key] = apiOptions[key];
                }
                return map;
            }, {});
            endpointDefaults.request = {
                validate: apiOptions.params
            };
            let request = octokit.request.defaults(endpointDefaults);
            // patch request & endpoint methods to support deprecated parameters.
            // Not the most elegant solution, but we don’t want to move deprecation
            // logic into octokit/endpoint.js as it’s out of scope
            const hasDeprecatedParam = Object.keys(apiOptions.params || {}).find(key => apiOptions.params[key].deprecated);
            if (hasDeprecatedParam) {
                const patch = patchForDeprecation.bind(null, octokit, apiOptions);
                request = patch(octokit.request.defaults(endpointDefaults), `.${namespaceName}.${apiName}()`);
                request.endpoint = patch(request.endpoint, `.${namespaceName}.${apiName}.endpoint()`);
                request.endpoint.merge = patch(request.endpoint.merge, `.${namespaceName}.${apiName}.endpoint.merge()`);
            }
            if (apiOptions.deprecated) {
                octokit[namespaceName][apiName] = Object.assign(function deprecatedEndpointMethod() {
                    octokit.log.warn(new Deprecation(`[@octokit/rest] ${apiOptions.deprecated}`));
                    octokit[namespaceName][apiName] = request;
                    return request.apply(null, arguments);
                }, request);
                return;
            }
            octokit[namespaceName][apiName] = request;
        });
    });
}
function patchForDeprecation(octokit, apiOptions, method, methodName) {
    const patchedMethod = (options) => {
        options = Object.assign({}, options);
        Object.keys(options).forEach(key => {
            if (apiOptions.params[key] && apiOptions.params[key].deprecated) {
                const aliasKey = apiOptions.params[key].alias;
                octokit.log.warn(new Deprecation(`[@octokit/rest] "${key}" parameter is deprecated for "${methodName}". Use "${aliasKey}" instead`));
                if (!(aliasKey in options)) {
                    options[aliasKey] = options[key];
                }
                delete options[key];
            }
        });
        return method(options);
    };
    Object.keys(method).forEach(key => {
        patchedMethod[key] = method[key];
    });
    return patchedMethod;
}
