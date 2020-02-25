declare module "domain" {
    import { EventEmitter } from "events";

    class Domain extends EventEmitter implements NodeJS.Domain {
        run<T>(fn: (...args: any[]) => T, ...args: any[]): T;
        add(emitter: EventEmitter | NodeJS.Timer): void;
        remove(emitter: EventEmitter | NodeJS.Timer): void;
        bind<T extends Function>(cb: T): T;
        intercept<T extends Function>(cb: T): T;
        members: Array<EventEmitter | NodeJS.Timer>;
        enter(): void;
        exit(): void;
    }

    function create(): Domain;
}
