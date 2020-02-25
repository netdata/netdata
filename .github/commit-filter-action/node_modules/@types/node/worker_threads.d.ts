declare module "worker_threads" {
    import { Context } from "vm";
    import { EventEmitter } from "events";
    import { Readable, Writable } from "stream";

    const isMainThread: boolean;
    const parentPort: null | MessagePort;
    const threadId: number;
    const workerData: any;

    class MessageChannel {
        readonly port1: MessagePort;
        readonly port2: MessagePort;
    }

    class MessagePort extends EventEmitter {
        close(): void;
        postMessage(value: any, transferList?: Array<ArrayBuffer | MessagePort>): void;
        ref(): void;
        unref(): void;
        start(): void;

        addListener(event: "close", listener: () => void): this;
        addListener(event: "message", listener: (value: any) => void): this;
        addListener(event: string | symbol, listener: (...args: any[]) => void): this;

        emit(event: "close"): boolean;
        emit(event: "message", value: any): boolean;
        emit(event: string | symbol, ...args: any[]): boolean;

        on(event: "close", listener: () => void): this;
        on(event: "message", listener: (value: any) => void): this;
        on(event: string | symbol, listener: (...args: any[]) => void): this;

        once(event: "close", listener: () => void): this;
        once(event: "message", listener: (value: any) => void): this;
        once(event: string | symbol, listener: (...args: any[]) => void): this;

        prependListener(event: "close", listener: () => void): this;
        prependListener(event: "message", listener: (value: any) => void): this;
        prependListener(event: string | symbol, listener: (...args: any[]) => void): this;

        prependOnceListener(event: "close", listener: () => void): this;
        prependOnceListener(event: "message", listener: (value: any) => void): this;
        prependOnceListener(event: string | symbol, listener: (...args: any[]) => void): this;

        removeListener(event: "close", listener: () => void): this;
        removeListener(event: "message", listener: (value: any) => void): this;
        removeListener(event: string | symbol, listener: (...args: any[]) => void): this;

        off(event: "close", listener: () => void): this;
        off(event: "message", listener: (value: any) => void): this;
        off(event: string | symbol, listener: (...args: any[]) => void): this;
    }

    interface WorkerOptions {
        /**
         * List of arguments which would be stringified and appended to
         * `process.argv` in the worker. This is mostly similar to the `workerData`
         * but the values will be available on the global `process.argv` as if they
         * were passed as CLI options to the script.
         */
        argv?: any[];
        eval?: boolean;
        workerData?: any;
        stdin?: boolean;
        stdout?: boolean;
        stderr?: boolean;
        execArgv?: string[];
        resourceLimits?: ResourceLimits;
    }

    interface ResourceLimits {
        maxYoungGenerationSizeMb?: number;
        maxOldGenerationSizeMb?: number;
        codeRangeSizeMb?: number;
    }

    class Worker extends EventEmitter {
        readonly stdin: Writable | null;
        readonly stdout: Readable;
        readonly stderr: Readable;
        readonly threadId: number;
        readonly resourceLimits?: ResourceLimits;

        constructor(filename: string, options?: WorkerOptions);

        postMessage(value: any, transferList?: Array<ArrayBuffer | MessagePort>): void;
        ref(): void;
        unref(): void;
        /**
         * Stop all JavaScript execution in the worker thread as soon as possible.
         * Returns a Promise for the exit code that is fulfilled when the `exit` event is emitted.
         */
        terminate(): Promise<number>;
        /**
         * Transfer a `MessagePort` to a different `vm` Context. The original `port`
         * object will be rendered unusable, and the returned `MessagePort` instance will
         * take its place.
         *
         * The returned `MessagePort` will be an object in the target context, and will
         * inherit from its global `Object` class. Objects passed to the
         * `port.onmessage()` listener will also be created in the target context
         * and inherit from its global `Object` class.
         *
         * However, the created `MessagePort` will no longer inherit from
         * `EventEmitter`, and only `port.onmessage()` can be used to receive
         * events using it.
         */
        moveMessagePortToContext(port: MessagePort, context: Context): MessagePort;

        /**
         * Receive a single message from a given `MessagePort`. If no message is available,
         * `undefined` is returned, otherwise an object with a single `message` property
         * that contains the message payload, corresponding to the oldest message in the
         * `MessagePort`’s queue.
         */
        receiveMessageOnPort(port: MessagePort): {} | undefined;

        /**
         * Returns a readable stream for a V8 snapshot of the current state of the Worker.
         * See [`v8.getHeapSnapshot()`][] for more details.
         *
         * If the Worker thread is no longer running, which may occur before the
         * [`'exit'` event][] is emitted, the returned `Promise` will be rejected
         * immediately with an [`ERR_WORKER_NOT_RUNNING`][] error
         */
        getHeapSnapshot(): Promise<Readable>;

        addListener(event: "error", listener: (err: Error) => void): this;
        addListener(event: "exit", listener: (exitCode: number) => void): this;
        addListener(event: "message", listener: (value: any) => void): this;
        addListener(event: "online", listener: () => void): this;
        addListener(event: string | symbol, listener: (...args: any[]) => void): this;

        emit(event: "error", err: Error): boolean;
        emit(event: "exit", exitCode: number): boolean;
        emit(event: "message", value: any): boolean;
        emit(event: "online"): boolean;
        emit(event: string | symbol, ...args: any[]): boolean;

        on(event: "error", listener: (err: Error) => void): this;
        on(event: "exit", listener: (exitCode: number) => void): this;
        on(event: "message", listener: (value: any) => void): this;
        on(event: "online", listener: () => void): this;
        on(event: string | symbol, listener: (...args: any[]) => void): this;

        once(event: "error", listener: (err: Error) => void): this;
        once(event: "exit", listener: (exitCode: number) => void): this;
        once(event: "message", listener: (value: any) => void): this;
        once(event: "online", listener: () => void): this;
        once(event: string | symbol, listener: (...args: any[]) => void): this;

        prependListener(event: "error", listener: (err: Error) => void): this;
        prependListener(event: "exit", listener: (exitCode: number) => void): this;
        prependListener(event: "message", listener: (value: any) => void): this;
        prependListener(event: "online", listener: () => void): this;
        prependListener(event: string | symbol, listener: (...args: any[]) => void): this;

        prependOnceListener(event: "error", listener: (err: Error) => void): this;
        prependOnceListener(event: "exit", listener: (exitCode: number) => void): this;
        prependOnceListener(event: "message", listener: (value: any) => void): this;
        prependOnceListener(event: "online", listener: () => void): this;
        prependOnceListener(event: string | symbol, listener: (...args: any[]) => void): this;

        removeListener(event: "error", listener: (err: Error) => void): this;
        removeListener(event: "exit", listener: (exitCode: number) => void): this;
        removeListener(event: "message", listener: (value: any) => void): this;
        removeListener(event: "online", listener: () => void): this;
        removeListener(event: string | symbol, listener: (...args: any[]) => void): this;

        off(event: "error", listener: (err: Error) => void): this;
        off(event: "exit", listener: (exitCode: number) => void): this;
        off(event: "message", listener: (value: any) => void): this;
        off(event: "online", listener: () => void): this;
        off(event: string | symbol, listener: (...args: any[]) => void): this;
    }
}
