import { Octokit } from "@octokit/core";
import { MapFunction, PaginationResults, RequestParameters, Route } from "./types";
export declare function paginate(octokit: Octokit, route: Route, parameters?: RequestParameters, mapFn?: MapFunction): Promise<PaginationResults<any>>;
