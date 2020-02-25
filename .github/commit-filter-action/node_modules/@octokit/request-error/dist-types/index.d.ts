import { RequestOptions, ResponseHeaders } from "@octokit/types";
import { RequestErrorOptions } from "./types";
/**
 * Error with extra properties to help with debugging
 */
export declare class RequestError extends Error {
    name: "HttpError";
    /**
     * http status code
     */
    status: number;
    /**
     * http status code
     *
     * @deprecated `error.code` is deprecated in favor of `error.status`
     */
    code: number;
    /**
     * error response headers
     */
    headers: ResponseHeaders;
    /**
     * Request options that lead to the error.
     */
    request: RequestOptions;
    constructor(message: string, statusCode: number, options: RequestErrorOptions);
}
