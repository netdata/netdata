import { graphql as GraphQL } from '@octokit/graphql/dist-types/types';
import { Octokit } from '@octokit/rest';
import * as Context from './context';
export declare const context: Context.Context;
export declare class GitHub extends Octokit {
    graphql: GraphQL;
    /**
     * Sets up the REST client and GraphQL client with auth and proxy support.
     * The parameter `token` or `opts.auth` must be supplied. The GraphQL client
     * authorization is not setup when `opts.auth` is a function or object.
     *
     * @param token  Auth token
     * @param opts   Octokit options
     */
    constructor(token: string, opts?: Omit<Octokit.Options, 'auth'>);
    constructor(opts: Octokit.Options);
    /**
     * Disambiguates the constructor overload parameters
     */
    private static disambiguate;
    private static getOctokitOptions;
    private static getGraphQL;
    private static getAuthString;
    private static getProxyAgent;
}
