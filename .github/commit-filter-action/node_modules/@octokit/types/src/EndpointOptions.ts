import { RequestMethod } from "./RequestMethod";
import { Url } from "./Url";
import { RequestParameters } from "./RequestParameters";

export type EndpointOptions = RequestParameters & {
  method: RequestMethod;
  url: Url;
};
