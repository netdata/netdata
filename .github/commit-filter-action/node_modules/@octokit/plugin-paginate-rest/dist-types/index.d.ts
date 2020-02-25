import { PaginateInterface } from "./types";
export { PaginateInterface } from "./types";
import { Octokit } from "@octokit/core";
/**
 * @param octokit Octokit instance
 * @param options Options passed to Octokit constructor
 */
export declare function paginateRest(octokit: Octokit): {
    paginate: PaginateInterface;
};
export declare namespace paginateRest {
    var VERSION: string;
}
