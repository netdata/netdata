declare module "repl" {
    import { Interface, Completer, AsyncCompleter } from "readline";
    import { Context } from "vm";
    import { InspectOptions } from "util";

    interface ReplOptions {
        /**
         * The input prompt to display.
         * Default: `"> "`
         */
        prompt?: string;
        /**
         * The `Readable` stream from which REPL input will be read.
         * Default: `process.stdin`
         */
        input?: NodeJS.ReadableStream;
        /**
         * The `Writable` stream to which REPL output will be written.
         * Default: `process.stdout`
         */
        output?: NodeJS.WritableStream;
        /**
         * If `true`, specifies that the output should be treated as a TTY terminal, and have
         * ANSI/VT100 escape codes written to it.
         * Default: checking the value of the `isTTY` property on the output stream upon
         * instantiation.
         */
        terminal?: boolean;
        /**
         * The function to be used when evaluating each given line of input.
         * Default: an async wrapper for the JavaScript `eval()` function. An `eval` function can
         * error with `repl.Recoverable` to indicate the input was incomplete and prompt for
         * additional lines.
         *
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_default_evaluation
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_custom_evaluation_functions
         */
        eval?: REPLEval;
        /**
         * Defines if the repl prints output previews or not.
         * @default `true` Always `false` in case `terminal` is falsy.
         */
        preview?: boolean;
        /**
         * If `true`, specifies that the default `writer` function should include ANSI color
         * styling to REPL output. If a custom `writer` function is provided then this has no
         * effect.
         * Default: the REPL instance's `terminal` value.
         */
        useColors?: boolean;
        /**
         * If `true`, specifies that the default evaluation function will use the JavaScript
         * `global` as the context as opposed to creating a new separate context for the REPL
         * instance. The node CLI REPL sets this value to `true`.
         * Default: `false`.
         */
        useGlobal?: boolean;
        /**
         * If `true`, specifies that the default writer will not output the return value of a
         * command if it evaluates to `undefined`.
         * Default: `false`.
         */
        ignoreUndefined?: boolean;
        /**
         * The function to invoke to format the output of each command before writing to `output`.
         * Default: a wrapper for `util.inspect`.
         *
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_customizing_repl_output
         */
        writer?: REPLWriter;
        /**
         * An optional function used for custom Tab auto completion.
         *
         * @see https://nodejs.org/dist/latest-v11.x/docs/api/readline.html#readline_use_of_the_completer_function
         */
        completer?: Completer | AsyncCompleter;
        /**
         * A flag that specifies whether the default evaluator executes all JavaScript commands in
         * strict mode or default (sloppy) mode.
         * Accepted values are:
         * - `repl.REPL_MODE_SLOPPY` - evaluates expressions in sloppy mode.
         * - `repl.REPL_MODE_STRICT` - evaluates expressions in strict mode. This is equivalent to
         *   prefacing every repl statement with `'use strict'`.
         */
        replMode?: typeof REPL_MODE_SLOPPY | typeof REPL_MODE_STRICT;
        /**
         * Stop evaluating the current piece of code when `SIGINT` is received, i.e. `Ctrl+C` is
         * pressed. This cannot be used together with a custom `eval` function.
         * Default: `false`.
         */
        breakEvalOnSigint?: boolean;
    }

    type REPLEval = (this: REPLServer, evalCmd: string, context: Context, file: string, cb: (err: Error | null, result: any) => void) => void;
    type REPLWriter = (this: REPLServer, obj: any) => string;

    /**
     * This is the default "writer" value, if none is passed in the REPL options,
     * and it can be overridden by custom print functions.
     */
    const writer: REPLWriter & { options: InspectOptions };

    type REPLCommandAction = (this: REPLServer, text: string) => void;

    interface REPLCommand {
        /**
         * Help text to be displayed when `.help` is entered.
         */
        help?: string;
        /**
         * The function to execute, optionally accepting a single string argument.
         */
        action: REPLCommandAction;
    }

    /**
     * Provides a customizable Read-Eval-Print-Loop (REPL).
     *
     * Instances of `repl.REPLServer` will accept individual lines of user input, evaluate those
     * according to a user-defined evaluation function, then output the result. Input and output
     * may be from `stdin` and `stdout`, respectively, or may be connected to any Node.js `stream`.
     *
     * Instances of `repl.REPLServer` support automatic completion of inputs, simplistic Emacs-style
     * line editing, multi-line inputs, ANSI-styled output, saving and restoring current REPL session
     * state, error recovery, and customizable evaluation functions.
     *
     * Instances of `repl.REPLServer` are created using the `repl.start()` method and _should not_
     * be created directly using the JavaScript `new` keyword.
     *
     * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_repl
     */
    class REPLServer extends Interface {
        /**
         * The `vm.Context` provided to the `eval` function to be used for JavaScript
         * evaluation.
         */
        readonly context: Context;
        /**
         * The `Readable` stream from which REPL input will be read.
         */
        readonly inputStream: NodeJS.ReadableStream;
        /**
         * The `Writable` stream to which REPL output will be written.
         */
        readonly outputStream: NodeJS.WritableStream;
        /**
         * The commands registered via `replServer.defineCommand()`.
         */
        readonly commands: { readonly [name: string]: REPLCommand | undefined };
        /**
         * A value indicating whether the REPL is currently in "editor mode".
         *
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_commands_and_special_keys
         */
        readonly editorMode: boolean;
        /**
         * A value indicating whether the `_` variable has been assigned.
         *
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_assignment_of_the_underscore_variable
         */
        readonly underscoreAssigned: boolean;
        /**
         * The last evaluation result from the REPL (assigned to the `_` variable inside of the REPL).
         *
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_assignment_of_the_underscore_variable
         */
        readonly last: any;
        /**
         * A value indicating whether the `_error` variable has been assigned.
         *
         * @since v9.8.0
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_assignment_of_the_underscore_variable
         */
        readonly underscoreErrAssigned: boolean;
        /**
         * The last error raised inside the REPL (assigned to the `_error` variable inside of the REPL).
         *
         * @since v9.8.0
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_assignment_of_the_underscore_variable
         */
        readonly lastError: any;
        /**
         * Specified in the REPL options, this is the function to be used when evaluating each
         * given line of input. If not specified in the REPL options, this is an async wrapper
         * for the JavaScript `eval()` function.
         */
        readonly eval: REPLEval;
        /**
         * Specified in the REPL options, this is a value indicating whether the default
         * `writer` function should include ANSI color styling to REPL output.
         */
        readonly useColors: boolean;
        /**
         * Specified in the REPL options, this is a value indicating whether the default `eval`
         * function will use the JavaScript `global` as the context as opposed to creating a new
         * separate context for the REPL instance.
         */
        readonly useGlobal: boolean;
        /**
         * Specified in the REPL options, this is a value indicating whether the default `writer`
         * function should output the result of a command if it evaluates to `undefined`.
         */
        readonly ignoreUndefined: boolean;
        /**
         * Specified in the REPL options, this is the function to invoke to format the output of
         * each command before writing to `outputStream`. If not specified in the REPL options,
         * this will be a wrapper for `util.inspect`.
         */
        readonly writer: REPLWriter;
        /**
         * Specified in the REPL options, this is the function to use for custom Tab auto-completion.
         */
        readonly completer: Completer | AsyncCompleter;
        /**
         * Specified in the REPL options, this is a flag that specifies whether the default `eval`
         * function should execute all JavaScript commands in strict mode or default (sloppy) mode.
         * Possible values are:
         * - `repl.REPL_MODE_SLOPPY` - evaluates expressions in sloppy mode.
         * - `repl.REPL_MODE_STRICT` - evaluates expressions in strict mode. This is equivalent to
         *    prefacing every repl statement with `'use strict'`.
         */
        readonly replMode: typeof REPL_MODE_SLOPPY | typeof REPL_MODE_STRICT;

        /**
         * NOTE: According to the documentation:
         *
         * > Instances of `repl.REPLServer` are created using the `repl.start()` method and
         * > _should not_ be created directly using the JavaScript `new` keyword.
         *
         * `REPLServer` cannot be subclassed due to implementation specifics in NodeJS.
         *
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_class_replserver
         */
        private constructor();

        /**
         * Used to add new `.`-prefixed commands to the REPL instance. Such commands are invoked
         * by typing a `.` followed by the `keyword`.
         *
         * @param keyword The command keyword (_without_ a leading `.` character).
         * @param cmd The function to invoke when the command is processed.
         *
         * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_replserver_definecommand_keyword_cmd
         */
        defineCommand(keyword: string, cmd: REPLCommandAction | REPLCommand): void;
        /**
         * Readies the REPL instance for input from the user, printing the configured `prompt` to a
         * new line in the `output` and resuming the `input` to accept new input.
         *
         * When multi-line input is being entered, an ellipsis is printed rather than the 'prompt'.
         *
         * This method is primarily intended to be called from within the action function for
         * commands registered using the `replServer.defineCommand()` method.
         *
         * @param preserveCursor When `true`, the cursor placement will not be reset to `0`.
         */
        displayPrompt(preserveCursor?: boolean): void;
        /**
         * Clears any command that has been buffered but not yet executed.
         *
         * This method is primarily intended to be called from within the action function for
         * commands registered using the `replServer.defineCommand()` method.
         *
         * @since v9.0.0
         */
        clearBufferedCommand(): void;

        /**
         * Initializes a history log file for the REPL instance. When executing the
         * Node.js binary and using the command line REPL, a history file is initialized
         * by default. However, this is not the case when creating a REPL
         * programmatically. Use this method to initialize a history log file when working
         * with REPL instances programmatically.
         * @param path The path to the history file
         */
        setupHistory(path: string, cb: (err: Error | null, repl: this) => void): void;

        /**
         * events.EventEmitter
         * 1. close - inherited from `readline.Interface`
         * 2. line - inherited from `readline.Interface`
         * 3. pause - inherited from `readline.Interface`
         * 4. resume - inherited from `readline.Interface`
         * 5. SIGCONT - inherited from `readline.Interface`
         * 6. SIGINT - inherited from `readline.Interface`
         * 7. SIGTSTP - inherited from `readline.Interface`
         * 8. exit
         * 9. reset
         */

        addListener(event: string, listener: (...args: any[]) => void): this;
        addListener(event: "close", listener: () => void): this;
        addListener(event: "line", listener: (input: string) => void): this;
        addListener(event: "pause", listener: () => void): this;
        addListener(event: "resume", listener: () => void): this;
        addListener(event: "SIGCONT", listener: () => void): this;
        addListener(event: "SIGINT", listener: () => void): this;
        addListener(event: "SIGTSTP", listener: () => void): this;
        addListener(event: "exit", listener: () => void): this;
        addListener(event: "reset", listener: (context: Context) => void): this;

        emit(event: string | symbol, ...args: any[]): boolean;
        emit(event: "close"): boolean;
        emit(event: "line", input: string): boolean;
        emit(event: "pause"): boolean;
        emit(event: "resume"): boolean;
        emit(event: "SIGCONT"): boolean;
        emit(event: "SIGINT"): boolean;
        emit(event: "SIGTSTP"): boolean;
        emit(event: "exit"): boolean;
        emit(event: "reset", context: Context): boolean;

        on(event: string, listener: (...args: any[]) => void): this;
        on(event: "close", listener: () => void): this;
        on(event: "line", listener: (input: string) => void): this;
        on(event: "pause", listener: () => void): this;
        on(event: "resume", listener: () => void): this;
        on(event: "SIGCONT", listener: () => void): this;
        on(event: "SIGINT", listener: () => void): this;
        on(event: "SIGTSTP", listener: () => void): this;
        on(event: "exit", listener: () => void): this;
        on(event: "reset", listener: (context: Context) => void): this;

        once(event: string, listener: (...args: any[]) => void): this;
        once(event: "close", listener: () => void): this;
        once(event: "line", listener: (input: string) => void): this;
        once(event: "pause", listener: () => void): this;
        once(event: "resume", listener: () => void): this;
        once(event: "SIGCONT", listener: () => void): this;
        once(event: "SIGINT", listener: () => void): this;
        once(event: "SIGTSTP", listener: () => void): this;
        once(event: "exit", listener: () => void): this;
        once(event: "reset", listener: (context: Context) => void): this;

        prependListener(event: string, listener: (...args: any[]) => void): this;
        prependListener(event: "close", listener: () => void): this;
        prependListener(event: "line", listener: (input: string) => void): this;
        prependListener(event: "pause", listener: () => void): this;
        prependListener(event: "resume", listener: () => void): this;
        prependListener(event: "SIGCONT", listener: () => void): this;
        prependListener(event: "SIGINT", listener: () => void): this;
        prependListener(event: "SIGTSTP", listener: () => void): this;
        prependListener(event: "exit", listener: () => void): this;
        prependListener(event: "reset", listener: (context: Context) => void): this;

        prependOnceListener(event: string, listener: (...args: any[]) => void): this;
        prependOnceListener(event: "close", listener: () => void): this;
        prependOnceListener(event: "line", listener: (input: string) => void): this;
        prependOnceListener(event: "pause", listener: () => void): this;
        prependOnceListener(event: "resume", listener: () => void): this;
        prependOnceListener(event: "SIGCONT", listener: () => void): this;
        prependOnceListener(event: "SIGINT", listener: () => void): this;
        prependOnceListener(event: "SIGTSTP", listener: () => void): this;
        prependOnceListener(event: "exit", listener: () => void): this;
        prependOnceListener(event: "reset", listener: (context: Context) => void): this;
    }

    /**
     * A flag passed in the REPL options. Evaluates expressions in sloppy mode.
     */
    const REPL_MODE_SLOPPY: unique symbol;

    /**
     * A flag passed in the REPL options. Evaluates expressions in strict mode.
     * This is equivalent to prefacing every repl statement with `'use strict'`.
     */
    const REPL_MODE_STRICT: unique symbol;

    /**
     * Creates and starts a `repl.REPLServer` instance.
     *
     * @param options The options for the `REPLServer`. If `options` is a string, then it specifies
     * the input prompt.
     */
    function start(options?: string | ReplOptions): REPLServer;

    /**
     * Indicates a recoverable error that a `REPLServer` can use to support multi-line input.
     *
     * @see https://nodejs.org/dist/latest-v10.x/docs/api/repl.html#repl_recoverable_errors
     */
    class Recoverable extends SyntaxError {
        err: Error;

        constructor(err: Error);
    }
}
