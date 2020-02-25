import { RequestHeaders } from "./RequestHeaders";
import { RequestMethod } from "./RequestMethod";
import { RequestRequestOptions } from "./RequestRequestOptions";
import { Url } from "./Url";

/**
 * Generic request options as they are returned by the `endpoint()` method
 */
export type RequestOptions = {
  method: RequestMethod;
  url: Url;
  headers: RequestHeaders;
  body?: any;
  request?: RequestRequestOptions;
};
