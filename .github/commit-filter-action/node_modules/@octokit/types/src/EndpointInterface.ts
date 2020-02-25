import { EndpointDefaults } from "./EndpointDefaults";
import { EndpointOptions } from "./EndpointOptions";
import { RequestOptions } from "./RequestOptions";
import { RequestParameters } from "./RequestParameters";
import { Route } from "./Route";

import { Endpoints } from "./generated/Endpoints";

export interface EndpointInterface {
  /**
   * Transforms a GitHub REST API endpoint into generic request options
   *
   * @param {object} endpoint Must set `method` and `url`. Plus URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
   */
  (options: EndpointOptions): RequestOptions;

  /**
   * Transforms a GitHub REST API endpoint into generic request options
   *
   * @param {string} route Request method + URL. Example: `'GET /orgs/:org'`
   * @param {object} [parameters] URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
   */
  <R extends Route>(
    route: keyof Endpoints | R,
    options?: R extends keyof Endpoints
      ? Endpoints[R][0] & RequestParameters
      : RequestParameters
  ): R extends keyof Endpoints ? Endpoints[R][1] : RequestOptions;

  /**
   * Object with current default route and parameters
   */
  DEFAULTS: EndpointDefaults;

  /**
   * Returns a new `endpoint` with updated route and parameters
   */
  defaults: (newDefaults: RequestParameters) => EndpointInterface;

  merge: {
    /**
     * Merges current endpoint defaults with passed route and parameters,
     * without transforming them into request options.
     *
     * @param {string} route Request method + URL. Example: `'GET /orgs/:org'`
     * @param {object} [parameters] URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
     *
     */
    (route: Route, parameters?: RequestParameters): EndpointDefaults;

    /**
     * Merges current endpoint defaults with passed route and parameters,
     * without transforming them into request options.
     *
     * @param {object} endpoint Must set `method` and `url`. Plus URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
     */
    (options: RequestParameters): EndpointDefaults;

    /**
     * Returns current default options.
     *
     * @deprecated use endpoint.DEFAULTS instead
     */
    (): EndpointDefaults;
  };

  /**
   * Stateless method to turn endpoint options into request options.
   * Calling `endpoint(options)` is the same as calling `endpoint.parse(endpoint.merge(options))`.
   *
   * @param {object} options `method`, `url`. Plus URL, query or body parameters, as well as `headers`, `mediaType.{format|previews}`, `request`, or `baseUrl`.
   */
  parse: (options: EndpointDefaults) => RequestOptions;
}
