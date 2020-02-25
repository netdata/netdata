import ifm = require('./interfaces');
export declare class BasicCredentialHandler implements ifm.IRequestHandler {
    username: string;
    password: string;
    constructor(username: string, password: string);
    prepareRequest(options: any): void;
    canHandleAuthentication(response: ifm.IHttpClientResponse): boolean;
    handleAuthentication(httpClient: ifm.IHttpClient, requestInfo: ifm.IRequestInfo, objs: any): Promise<ifm.IHttpClientResponse>;
}
export declare class BearerCredentialHandler implements ifm.IRequestHandler {
    token: string;
    constructor(token: string);
    prepareRequest(options: any): void;
    canHandleAuthentication(response: ifm.IHttpClientResponse): boolean;
    handleAuthentication(httpClient: ifm.IHttpClient, requestInfo: ifm.IRequestInfo, objs: any): Promise<ifm.IHttpClientResponse>;
}
export declare class PersonalAccessTokenCredentialHandler implements ifm.IRequestHandler {
    token: string;
    constructor(token: string);
    prepareRequest(options: any): void;
    canHandleAuthentication(response: ifm.IHttpClientResponse): boolean;
    handleAuthentication(httpClient: ifm.IHttpClient, requestInfo: ifm.IRequestInfo, objs: any): Promise<ifm.IHttpClientResponse>;
}
