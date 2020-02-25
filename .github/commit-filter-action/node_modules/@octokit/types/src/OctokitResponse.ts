import { ResponseHeaders } from "./ResponseHeaders";
import { Url } from "./Url";

export type OctokitResponse<T> = {
  headers: ResponseHeaders;
  /**
   * http response code
   */
  status: number;
  /**
   * URL of response after all redirects
   */
  url: Url;
  /**
   *  This is the data you would see in https://developer.Octokit.com/v3/
   */
  data: T;
};
