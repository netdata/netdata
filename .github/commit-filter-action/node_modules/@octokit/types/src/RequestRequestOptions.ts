import { Agent } from "http";
import { Fetch } from "./Fetch";
import { Signal } from "./Signal";

/**
 * Octokit-specific request options which are ignored for the actual request, but can be used by Octokit or plugins to manipulate how the request is sent or how a response is handled
 */
export type RequestRequestOptions = {
  /**
   * Node only. Useful for custom proxy, certificate, or dns lookup.
   */
  agent?: Agent;
  /**
   * Custom replacement for built-in fetch method. Useful for testing or request hooks.
   */
  fetch?: Fetch;
  /**
   * Use an `AbortController` instance to cancel a request. In node you can only cancel streamed requests.
   */
  signal?: Signal;
  /**
   * Node only. Request/response timeout in ms, it resets on redirect. 0 to disable (OS limit applies). `options.request.signal` is recommended instead.
   */
  timeout?: number;

  [option: string]: any;
};
