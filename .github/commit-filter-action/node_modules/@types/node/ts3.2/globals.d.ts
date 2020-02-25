// tslint:disable-next-line:no-bad-reference
/// <reference path="../globals.d.ts" />

declare namespace NodeJS {
    interface HRTime {
        bigint(): bigint;
    }
}

interface Buffer extends Uint8Array {
    readBigUInt64BE(offset?: number): bigint;
    readBigUInt64LE(offset?: number): bigint;
    readBigInt64BE(offset?: number): bigint;
    readBigInt64LE(offset?: number): bigint;
    writeBigInt64BE(value: bigint, offset?: number): number;
    writeBigInt64LE(value: bigint, offset?: number): number;
    writeBigUInt64BE(value: bigint, offset?: number): number;
    writeBigUInt64LE(value: bigint, offset?: number): number;
}
