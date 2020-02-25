declare module "tty" {
    import * as net from "net";

    function isatty(fd: number): boolean;
    class ReadStream extends net.Socket {
        constructor(fd: number, options?: net.SocketConstructorOpts);
        isRaw: boolean;
        setRawMode(mode: boolean): this;
        isTTY: boolean;
    }
    /**
     * -1 - to the left from cursor
     *  0 - the entire line
     *  1 - to the right from cursor
     */
    type Direction = -1 | 0 | 1;
    class WriteStream extends net.Socket {
        constructor(fd: number);
        addListener(event: string, listener: (...args: any[]) => void): this;
        addListener(event: "resize", listener: () => void): this;

        emit(event: string | symbol, ...args: any[]): boolean;
        emit(event: "resize"): boolean;

        on(event: string, listener: (...args: any[]) => void): this;
        on(event: "resize", listener: () => void): this;

        once(event: string, listener: (...args: any[]) => void): this;
        once(event: "resize", listener: () => void): this;

        prependListener(event: string, listener: (...args: any[]) => void): this;
        prependListener(event: "resize", listener: () => void): this;

        prependOnceListener(event: string, listener: (...args: any[]) => void): this;
        prependOnceListener(event: "resize", listener: () => void): this;

        /**
         * Clears the current line of this WriteStream in a direction identified by `dir`.
         */
        clearLine(dir: Direction, callback?: () => void): boolean;
        /**
         * Clears this `WriteStream` from the current cursor down.
         */
        clearScreenDown(callback?: () => void): boolean;
        /**
         * Moves this WriteStream's cursor to the specified position.
         */
        cursorTo(x: number, y?: number, callback?: () => void): boolean;
        cursorTo(x: number, callback: () => void): boolean;
        /**
         * Moves this WriteStream's cursor relative to its current position.
         */
        moveCursor(dx: number, dy: number, callback?: () => void): boolean;
        /**
         * @default `process.env`
         */
        getColorDepth(env?: {}): number;
        hasColors(depth?: number): boolean;
        hasColors(env?: {}): boolean;
        hasColors(depth: number, env?: {}): boolean;
        getWindowSize(): [number, number];
        columns: number;
        rows: number;
        isTTY: boolean;
    }
}
