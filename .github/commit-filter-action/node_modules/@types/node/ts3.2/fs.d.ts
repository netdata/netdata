// tslint:disable-next-line:no-bad-reference
/// <reference path="../fs.d.ts" />

declare module 'fs' {
    interface BigIntStats extends StatsBase<BigInt> {
    }

    class BigIntStats {
        atimeNs: BigInt;
        mtimeNs: BigInt;
        ctimeNs: BigInt;
        birthtimeNs: BigInt;
    }

    interface BigIntOptions {
        bigint: true;
    }

    interface StatOptions {
        bigint: boolean;
    }

    function stat(path: PathLike, options: BigIntOptions, callback: (err: NodeJS.ErrnoException | null, stats: BigIntStats) => void): void;
    function stat(path: PathLike, options: StatOptions, callback: (err: NodeJS.ErrnoException | null, stats: Stats | BigIntStats) => void): void;

    namespace stat {
        function __promisify__(path: PathLike, options: BigIntOptions): Promise<BigIntStats>;
        function __promisify__(path: PathLike, options: StatOptions): Promise<Stats | BigIntStats>;
    }

    function statSync(path: PathLike, options: BigIntOptions): BigIntStats;
    function statSync(path: PathLike, options: StatOptions): Stats | BigIntStats;
}
