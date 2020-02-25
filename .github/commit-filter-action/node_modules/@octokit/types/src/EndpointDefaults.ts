import { RequestHeaders } from "./RequestHeaders";
import { RequestMethod } from "./RequestMethod";
import { RequestParameters } from "./RequestParameters";
import { Url } from "./Url";

/**
 * The `.endpoint()` method is guaranteed to set all keys defined by RequestParameters
 * as well as the method property.
 */
export type EndpointDefaults = RequestParameters & {
  baseUrl: Url;
  method: RequestMethod;
  url?: Url;
  headers: RequestHeaders & {
    accept: string;
    "user-agent": string;
  };
  mediaType: {
    format: string;
    previews: string[];
  };
};
