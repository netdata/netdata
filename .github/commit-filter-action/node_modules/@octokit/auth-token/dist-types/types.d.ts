import * as OctokitTypes from "@octokit/types";
export declare type AnyResponse = OctokitTypes.OctokitResponse<any>;
export declare type StrategyInterface = OctokitTypes.StrategyInterface<[Token], [], Authentication>;
export declare type EndpointDefaults = OctokitTypes.EndpointDefaults;
export declare type EndpointOptions = OctokitTypes.EndpointOptions;
export declare type RequestParameters = OctokitTypes.RequestParameters;
export declare type RequestInterface = OctokitTypes.RequestInterface;
export declare type Route = OctokitTypes.Route;
export declare type Token = string;
export declare type OAuthTokenAuthentication = {
    type: "token";
    tokenType: "oauth";
    token: Token;
};
export declare type InstallationTokenAuthentication = {
    type: "token";
    tokenType: "installation";
    token: Token;
};
export declare type AppAuthentication = {
    type: "token";
    tokenType: "app";
    token: Token;
};
export declare type Authentication = OAuthTokenAuthentication | InstallationTokenAuthentication | AppAuthentication;
