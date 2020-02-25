declare module "trace_events" {
    /**
     * The `Tracing` object is used to enable or disable tracing for sets of
     * categories. Instances are created using the
     * `trace_events.createTracing()` method.
     *
     * When created, the `Tracing` object is disabled. Calling the
     * `tracing.enable()` method adds the categories to the set of enabled trace
     * event categories. Calling `tracing.disable()` will remove the categories
     * from the set of enabled trace event categories.
     */
    interface Tracing {
        /**
         * A comma-separated list of the trace event categories covered by this
         * `Tracing` object.
         */
        readonly categories: string;

        /**
         * Disables this `Tracing` object.
         *
         * Only trace event categories _not_ covered by other enabled `Tracing`
         * objects and _not_ specified by the `--trace-event-categories` flag
         * will be disabled.
         */
        disable(): void;

        /**
         * Enables this `Tracing` object for the set of categories covered by
         * the `Tracing` object.
         */
        enable(): void;

        /**
         * `true` only if the `Tracing` object has been enabled.
         */
        readonly enabled: boolean;
    }

    interface CreateTracingOptions {
        /**
         * An array of trace category names. Values included in the array are
         * coerced to a string when possible. An error will be thrown if the
         * value cannot be coerced.
         */
        categories: string[];
    }

    /**
     * Creates and returns a Tracing object for the given set of categories.
     */
    function createTracing(options: CreateTracingOptions): Tracing;

    /**
     * Returns a comma-separated list of all currently-enabled trace event
     * categories. The current set of enabled trace event categories is
     * determined by the union of all currently-enabled `Tracing` objects and
     * any categories enabled using the `--trace-event-categories` flag.
     */
    function getEnabledCategories(): string | undefined;
}
