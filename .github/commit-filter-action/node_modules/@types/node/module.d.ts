declare module "module" {
    import { URL } from "url";
    namespace Module {
        /**
         * Updates all the live bindings for builtin ES Modules to match the properties of the CommonJS exports.
         * It does not add or remove exported names from the ES Modules.
         */
        function syncBuiltinESMExports(): void;

        /**
         * @experimental
         */
        function findSourceMap(path: string, error?: Error): SourceMap;
        interface SourceMapPayload {
            file: string;
            version: number;
            sources: string[];
            sourcesContent: string[];
            names: string[];
            mappings: string;
            sourceRoot: string;
        }

        interface SourceMapping {
            generatedLine: number;
            generatedColumn: number;
            originalSource: string;
            originalLine: number;
            originalColumn: number;
        }

        /**
         * @experimental
         */
        class SourceMap {
            readonly payload: SourceMapPayload;
            constructor(payload: SourceMapPayload);
            findEntry(line: number, column: number): SourceMapping;
        }
    }
    interface Module extends NodeModule {}
    class Module {
        static runMain(): void;
        static wrap(code: string): string;

        /**
         * @deprecated Deprecated since: v12.2.0. Please use createRequire() instead.
         */
        static createRequireFromPath(path: string): NodeRequire;
        static createRequire(path: string | URL): NodeRequire;
        static builtinModules: string[];

        static Module: typeof Module;

        constructor(id: string, parent?: Module);
    }
    export = Module;
}
