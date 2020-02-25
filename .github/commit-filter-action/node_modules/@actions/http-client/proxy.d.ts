/// <reference types="node" />
import * as url from 'url';
export declare function getProxyUrl(reqUrl: url.Url): url.Url | undefined;
export declare function checkBypass(reqUrl: url.Url): boolean;
