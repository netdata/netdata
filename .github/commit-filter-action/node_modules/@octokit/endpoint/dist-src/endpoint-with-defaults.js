import { merge } from "./merge";
import { parse } from "./parse";
export function endpointWithDefaults(defaults, route, options) {
    return parse(merge(defaults, route, options));
}
