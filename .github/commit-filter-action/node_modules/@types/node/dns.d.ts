declare module "dns" {
    // Supported getaddrinfo flags.
    const ADDRCONFIG: number;
    const V4MAPPED: number;

    interface LookupOptions {
        family?: number;
        hints?: number;
        all?: boolean;
        verbatim?: boolean;
    }

    interface LookupOneOptions extends LookupOptions {
        all?: false;
    }

    interface LookupAllOptions extends LookupOptions {
        all: true;
    }

    interface LookupAddress {
        address: string;
        family: number;
    }

    function lookup(hostname: string, family: number, callback: (err: NodeJS.ErrnoException | null, address: string, family: number) => void): void;
    function lookup(hostname: string, options: LookupOneOptions, callback: (err: NodeJS.ErrnoException | null, address: string, family: number) => void): void;
    function lookup(hostname: string, options: LookupAllOptions, callback: (err: NodeJS.ErrnoException | null, addresses: LookupAddress[]) => void): void;
    function lookup(hostname: string, options: LookupOptions, callback: (err: NodeJS.ErrnoException | null, address: string | LookupAddress[], family: number) => void): void;
    function lookup(hostname: string, callback: (err: NodeJS.ErrnoException | null, address: string, family: number) => void): void;

    // NOTE: This namespace provides design-time support for util.promisify. Exported members do not exist at runtime.
    namespace lookup {
        function __promisify__(hostname: string, options: LookupAllOptions): Promise<LookupAddress[]>;
        function __promisify__(hostname: string, options?: LookupOneOptions | number): Promise<LookupAddress>;
        function __promisify__(hostname: string, options: LookupOptions): Promise<LookupAddress | LookupAddress[]>;
    }

    function lookupService(address: string, port: number, callback: (err: NodeJS.ErrnoException | null, hostname: string, service: string) => void): void;

    namespace lookupService {
        function __promisify__(address: string, port: number): Promise<{ hostname: string, service: string }>;
    }

    interface ResolveOptions {
        ttl: boolean;
    }

    interface ResolveWithTtlOptions extends ResolveOptions {
        ttl: true;
    }

    interface RecordWithTtl {
        address: string;
        ttl: number;
    }

    /** @deprecated Use AnyARecord or AnyAaaaRecord instead. */
    type AnyRecordWithTtl = AnyARecord | AnyAaaaRecord;

    interface AnyARecord extends RecordWithTtl {
        type: "A";
    }

    interface AnyAaaaRecord extends RecordWithTtl {
        type: "AAAA";
    }

    interface MxRecord {
        priority: number;
        exchange: string;
    }

    interface AnyMxRecord extends MxRecord {
        type: "MX";
    }

    interface NaptrRecord {
        flags: string;
        service: string;
        regexp: string;
        replacement: string;
        order: number;
        preference: number;
    }

    interface AnyNaptrRecord extends NaptrRecord {
        type: "NAPTR";
    }

    interface SoaRecord {
        nsname: string;
        hostmaster: string;
        serial: number;
        refresh: number;
        retry: number;
        expire: number;
        minttl: number;
    }

    interface AnySoaRecord extends SoaRecord {
        type: "SOA";
    }

    interface SrvRecord {
        priority: number;
        weight: number;
        port: number;
        name: string;
    }

    interface AnySrvRecord extends SrvRecord {
        type: "SRV";
    }

    interface AnyTxtRecord {
        type: "TXT";
        entries: string[];
    }

    interface AnyNsRecord {
        type: "NS";
        value: string;
    }

    interface AnyPtrRecord {
        type: "PTR";
        value: string;
    }

    interface AnyCnameRecord {
        type: "CNAME";
        value: string;
    }

    type AnyRecord = AnyARecord |
        AnyAaaaRecord |
        AnyCnameRecord |
        AnyMxRecord |
        AnyNaptrRecord |
        AnyNsRecord |
        AnyPtrRecord |
        AnySoaRecord |
        AnySrvRecord |
        AnyTxtRecord;

    function resolve(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    function resolve(hostname: string, rrtype: "A", callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    function resolve(hostname: string, rrtype: "AAAA", callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    function resolve(hostname: string, rrtype: "ANY", callback: (err: NodeJS.ErrnoException | null, addresses: AnyRecord[]) => void): void;
    function resolve(hostname: string, rrtype: "CNAME", callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    function resolve(hostname: string, rrtype: "MX", callback: (err: NodeJS.ErrnoException | null, addresses: MxRecord[]) => void): void;
    function resolve(hostname: string, rrtype: "NAPTR", callback: (err: NodeJS.ErrnoException | null, addresses: NaptrRecord[]) => void): void;
    function resolve(hostname: string, rrtype: "NS", callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    function resolve(hostname: string, rrtype: "PTR", callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    function resolve(hostname: string, rrtype: "SOA", callback: (err: NodeJS.ErrnoException | null, addresses: SoaRecord) => void): void;
    function resolve(hostname: string, rrtype: "SRV", callback: (err: NodeJS.ErrnoException | null, addresses: SrvRecord[]) => void): void;
    function resolve(hostname: string, rrtype: "TXT", callback: (err: NodeJS.ErrnoException | null, addresses: string[][]) => void): void;
    function resolve(
        hostname: string,
        rrtype: string,
        callback: (err: NodeJS.ErrnoException | null, addresses: string[] | MxRecord[] | NaptrRecord[] | SoaRecord | SrvRecord[] | string[][] | AnyRecord[]) => void,
    ): void;

    // NOTE: This namespace provides design-time support for util.promisify. Exported members do not exist at runtime.
    namespace resolve {
        function __promisify__(hostname: string, rrtype?: "A" | "AAAA" | "CNAME" | "NS" | "PTR"): Promise<string[]>;
        function __promisify__(hostname: string, rrtype: "ANY"): Promise<AnyRecord[]>;
        function __promisify__(hostname: string, rrtype: "MX"): Promise<MxRecord[]>;
        function __promisify__(hostname: string, rrtype: "NAPTR"): Promise<NaptrRecord[]>;
        function __promisify__(hostname: string, rrtype: "SOA"): Promise<SoaRecord>;
        function __promisify__(hostname: string, rrtype: "SRV"): Promise<SrvRecord[]>;
        function __promisify__(hostname: string, rrtype: "TXT"): Promise<string[][]>;
        function __promisify__(hostname: string, rrtype: string): Promise<string[] | MxRecord[] | NaptrRecord[] | SoaRecord | SrvRecord[] | string[][] | AnyRecord[]>;
    }

    function resolve4(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    function resolve4(hostname: string, options: ResolveWithTtlOptions, callback: (err: NodeJS.ErrnoException | null, addresses: RecordWithTtl[]) => void): void;
    function resolve4(hostname: string, options: ResolveOptions, callback: (err: NodeJS.ErrnoException | null, addresses: string[] | RecordWithTtl[]) => void): void;

    // NOTE: This namespace provides design-time support for util.promisify. Exported members do not exist at runtime.
    namespace resolve4 {
        function __promisify__(hostname: string): Promise<string[]>;
        function __promisify__(hostname: string, options: ResolveWithTtlOptions): Promise<RecordWithTtl[]>;
        function __promisify__(hostname: string, options?: ResolveOptions): Promise<string[] | RecordWithTtl[]>;
    }

    function resolve6(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    function resolve6(hostname: string, options: ResolveWithTtlOptions, callback: (err: NodeJS.ErrnoException | null, addresses: RecordWithTtl[]) => void): void;
    function resolve6(hostname: string, options: ResolveOptions, callback: (err: NodeJS.ErrnoException | null, addresses: string[] | RecordWithTtl[]) => void): void;

    // NOTE: This namespace provides design-time support for util.promisify. Exported members do not exist at runtime.
    namespace resolve6 {
        function __promisify__(hostname: string): Promise<string[]>;
        function __promisify__(hostname: string, options: ResolveWithTtlOptions): Promise<RecordWithTtl[]>;
        function __promisify__(hostname: string, options?: ResolveOptions): Promise<string[] | RecordWithTtl[]>;
    }

    function resolveCname(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    namespace resolveCname {
        function __promisify__(hostname: string): Promise<string[]>;
    }

    function resolveMx(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: MxRecord[]) => void): void;
    namespace resolveMx {
        function __promisify__(hostname: string): Promise<MxRecord[]>;
    }

    function resolveNaptr(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: NaptrRecord[]) => void): void;
    namespace resolveNaptr {
        function __promisify__(hostname: string): Promise<NaptrRecord[]>;
    }

    function resolveNs(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    namespace resolveNs {
        function __promisify__(hostname: string): Promise<string[]>;
    }

    function resolvePtr(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: string[]) => void): void;
    namespace resolvePtr {
        function __promisify__(hostname: string): Promise<string[]>;
    }

    function resolveSoa(hostname: string, callback: (err: NodeJS.ErrnoException | null, address: SoaRecord) => void): void;
    namespace resolveSoa {
        function __promisify__(hostname: string): Promise<SoaRecord>;
    }

    function resolveSrv(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: SrvRecord[]) => void): void;
    namespace resolveSrv {
        function __promisify__(hostname: string): Promise<SrvRecord[]>;
    }

    function resolveTxt(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: string[][]) => void): void;
    namespace resolveTxt {
        function __promisify__(hostname: string): Promise<string[][]>;
    }

    function resolveAny(hostname: string, callback: (err: NodeJS.ErrnoException | null, addresses: AnyRecord[]) => void): void;
    namespace resolveAny {
        function __promisify__(hostname: string): Promise<AnyRecord[]>;
    }

    function reverse(ip: string, callback: (err: NodeJS.ErrnoException | null, hostnames: string[]) => void): void;
    function setServers(servers: ReadonlyArray<string>): void;
    function getServers(): string[];

    // Error codes
    const NODATA: string;
    const FORMERR: string;
    const SERVFAIL: string;
    const NOTFOUND: string;
    const NOTIMP: string;
    const REFUSED: string;
    const BADQUERY: string;
    const BADNAME: string;
    const BADFAMILY: string;
    const BADRESP: string;
    const CONNREFUSED: string;
    const TIMEOUT: string;
    const EOF: string;
    const FILE: string;
    const NOMEM: string;
    const DESTRUCTION: string;
    const BADSTR: string;
    const BADFLAGS: string;
    const NONAME: string;
    const BADHINTS: string;
    const NOTINITIALIZED: string;
    const LOADIPHLPAPI: string;
    const ADDRGETNETWORKPARAMS: string;
    const CANCELLED: string;

    class Resolver {
        getServers: typeof getServers;
        setServers: typeof setServers;
        resolve: typeof resolve;
        resolve4: typeof resolve4;
        resolve6: typeof resolve6;
        resolveAny: typeof resolveAny;
        resolveCname: typeof resolveCname;
        resolveMx: typeof resolveMx;
        resolveNaptr: typeof resolveNaptr;
        resolveNs: typeof resolveNs;
        resolvePtr: typeof resolvePtr;
        resolveSoa: typeof resolveSoa;
        resolveSrv: typeof resolveSrv;
        resolveTxt: typeof resolveTxt;
        reverse: typeof reverse;
        cancel(): void;
    }

    namespace promises {
        function getServers(): string[];

        function lookup(hostname: string, family: number): Promise<LookupAddress>;
        function lookup(hostname: string, options: LookupOneOptions): Promise<LookupAddress>;
        function lookup(hostname: string, options: LookupAllOptions): Promise<LookupAddress[]>;
        function lookup(hostname: string, options: LookupOptions): Promise<LookupAddress | LookupAddress[]>;
        function lookup(hostname: string): Promise<LookupAddress>;

        function lookupService(address: string, port: number): Promise<{ hostname: string, service: string }>;

        function resolve(hostname: string): Promise<string[]>;
        function resolve(hostname: string, rrtype: "A"): Promise<string[]>;
        function resolve(hostname: string, rrtype: "AAAA"): Promise<string[]>;
        function resolve(hostname: string, rrtype: "ANY"): Promise<AnyRecord[]>;
        function resolve(hostname: string, rrtype: "CNAME"): Promise<string[]>;
        function resolve(hostname: string, rrtype: "MX"): Promise<MxRecord[]>;
        function resolve(hostname: string, rrtype: "NAPTR"): Promise<NaptrRecord[]>;
        function resolve(hostname: string, rrtype: "NS"): Promise<string[]>;
        function resolve(hostname: string, rrtype: "PTR"): Promise<string[]>;
        function resolve(hostname: string, rrtype: "SOA"): Promise<SoaRecord>;
        function resolve(hostname: string, rrtype: "SRV"): Promise<SrvRecord[]>;
        function resolve(hostname: string, rrtype: "TXT"): Promise<string[][]>;
        function resolve(hostname: string, rrtype: string): Promise<string[] | MxRecord[] | NaptrRecord[] | SoaRecord | SrvRecord[] | string[][] | AnyRecord[]>;

        function resolve4(hostname: string): Promise<string[]>;
        function resolve4(hostname: string, options: ResolveWithTtlOptions): Promise<RecordWithTtl[]>;
        function resolve4(hostname: string, options: ResolveOptions): Promise<string[] | RecordWithTtl[]>;

        function resolve6(hostname: string): Promise<string[]>;
        function resolve6(hostname: string, options: ResolveWithTtlOptions): Promise<RecordWithTtl[]>;
        function resolve6(hostname: string, options: ResolveOptions): Promise<string[] | RecordWithTtl[]>;

        function resolveAny(hostname: string): Promise<AnyRecord[]>;

        function resolveCname(hostname: string): Promise<string[]>;

        function resolveMx(hostname: string): Promise<MxRecord[]>;

        function resolveNaptr(hostname: string): Promise<NaptrRecord[]>;

        function resolveNs(hostname: string): Promise<string[]>;

        function resolvePtr(hostname: string): Promise<string[]>;

        function resolveSoa(hostname: string): Promise<SoaRecord>;

        function resolveSrv(hostname: string): Promise<SrvRecord[]>;

        function resolveTxt(hostname: string): Promise<string[][]>;

        function reverse(ip: string): Promise<string[]>;

        function setServers(servers: ReadonlyArray<string>): void;

        class Resolver {
            getServers: typeof getServers;
            resolve: typeof resolve;
            resolve4: typeof resolve4;
            resolve6: typeof resolve6;
            resolveAny: typeof resolveAny;
            resolveCname: typeof resolveCname;
            resolveMx: typeof resolveMx;
            resolveNaptr: typeof resolveNaptr;
            resolveNs: typeof resolveNs;
            resolvePtr: typeof resolvePtr;
            resolveSoa: typeof resolveSoa;
            resolveSrv: typeof resolveSrv;
            resolveTxt: typeof resolveTxt;
            reverse: typeof reverse;
            setServers: typeof setServers;
        }
    }
}
