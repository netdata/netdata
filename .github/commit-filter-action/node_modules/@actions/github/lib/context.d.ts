import { WebhookPayload } from './interfaces';
export declare class Context {
    /**
     * Webhook payload object that triggered the workflow
     */
    payload: WebhookPayload;
    eventName: string;
    sha: string;
    ref: string;
    workflow: string;
    action: string;
    actor: string;
    /**
     * Hydrate the context from the environment
     */
    constructor();
    get issue(): {
        owner: string;
        repo: string;
        number: number;
    };
    get repo(): {
        owner: string;
        repo: string;
    };
}
