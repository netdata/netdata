import * as OctokitTypes from "@octokit/types";
export { EndpointOptions } from "@octokit/types";
export { OctokitResponse } from "@octokit/types";
export { RequestParameters } from "@octokit/types";
export { Route } from "@octokit/types";
export interface PaginateInterface {
    /**
     * Sends a request based on endpoint options
     *
     * @param {object} endpoint Must set `method` and `url`. Plus URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
     * @param {function} mapFn Optional method to map each response to a custom array
     */
    <T, R>(options: OctokitTypes.EndpointOptions, mapFn: MapFunction<T, R>): Promise<PaginationResults<R>>;
    /**
     * Sends a request based on endpoint options
     *
     * @param {object} endpoint Must set `method` and `url`. Plus URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
     */
    <T>(options: OctokitTypes.EndpointOptions): Promise<PaginationResults<T>>;
    /**
     * Sends a request based on endpoint options
     *
     * @param {string} route Request method + URL. Example: `'GET /orgs/:org'`
     * @param {function} mapFn Optional method to map each response to a custom array
     */
    <T, R>(route: OctokitTypes.Route, mapFn: MapFunction<T>): Promise<PaginationResults<R>>;
    /**
     * Sends a request based on endpoint options
     *
     * @param {string} route Request method + URL. Example: `'GET /orgs/:org'`
     * @param {object} parameters URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
     * @param {function} mapFn Optional method to map each response to a custom array
     */
    <T, R>(route: OctokitTypes.Route, parameters: OctokitTypes.RequestParameters, mapFn: MapFunction<T>): Promise<PaginationResults<R>>;
    /**
     * Sends a request based on endpoint options
     *
     * @param {string} route Request method + URL. Example: `'GET /orgs/:org'`
     * @param {object} parameters URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
     */
    <T>(route: OctokitTypes.Route, parameters: OctokitTypes.RequestParameters): Promise<PaginationResults<T>>;
    /**
     * Sends a request based on endpoint options
     *
     * @param {string} route Request method + URL. Example: `'GET /orgs/:org'`
     */
    <T>(route: OctokitTypes.Route): Promise<PaginationResults<T>>;
    iterator: {
        /**
         * Get an asynchronous iterator for use with `for await()`,
         *
         * @see {link https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/for-await...of} for await...of
         * @param {object} endpoint Must set `method` and `url`. Plus URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
         */
        <T>(EndpointOptions: OctokitTypes.EndpointOptions): AsyncIterableIterator<OctokitTypes.OctokitResponse<PaginationResults<T>>>;
        /**
         * Get an asynchronous iterator for use with `for await()`,
         *
         * @see {link https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Statements/for-await...of} for await...of
         * @param {string} route Request method + URL. Example: `'GET /orgs/:org'`
         * @param {object} [parameters] URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
         */
        <T>(route: OctokitTypes.Route, parameters?: OctokitTypes.RequestParameters): AsyncIterableIterator<OctokitTypes.OctokitResponse<PaginationResults<T>>>;
    };
}
export interface MapFunction<T = any, R = any> {
    (response: OctokitTypes.OctokitResponse<PaginationResults<T>>, done: () => void): R[];
}
export declare type PaginationResults<T = any> = T[];
