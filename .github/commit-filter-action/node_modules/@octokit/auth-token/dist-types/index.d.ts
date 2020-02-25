import { StrategyInterface, Token, Authentication } from "./types";
export declare type Types = {
    StrategyOptions: Token;
    AuthOptions: never;
    Authentication: Authentication;
};
export declare const createTokenAuth: StrategyInterface;
