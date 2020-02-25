import { RestEndpointMethods } from "./generated/rest-endpoint-methods-types";
export declare type Api = {
    registerEndpoints: (endpoints: any) => void;
} & RestEndpointMethods;
