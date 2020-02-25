declare module "tls" {
    import * as crypto from "crypto";
    import * as dns from "dns";
    import * as net from "net";
    import * as stream from "stream";

    const CLIENT_RENEG_LIMIT: number;
    const CLIENT_RENEG_WINDOW: number;

    interface Certificate {
        /**
         * Country code.
         */
        C: string;
        /**
         * Street.
         */
        ST: string;
        /**
         * Locality.
         */
        L: string;
        /**
         * Organization.
         */
        O: string;
        /**
         * Organizational unit.
         */
        OU: string;
        /**
         * Common name.
         */
        CN: string;
    }

    interface PeerCertificate {
        subject: Certificate;
        issuer: Certificate;
        subjectaltname: string;
        infoAccess: { [index: string]: string[] | undefined };
        modulus: string;
        exponent: string;
        valid_from: string;
        valid_to: string;
        fingerprint: string;
        ext_key_usage: string[];
        serialNumber: string;
        raw: Buffer;
    }

    interface DetailedPeerCertificate extends PeerCertificate {
        issuerCertificate: DetailedPeerCertificate;
    }

    interface CipherNameAndProtocol {
        /**
         * The cipher name.
         */
        name: string;
        /**
         * SSL/TLS protocol version.
         */
        version: string;

        /**
         * IETF name for the cipher suite.
         */
        standardName: string;
    }

    interface EphemeralKeyInfo {
        /**
         * The supported types are 'DH' and 'ECDH'.
         */
        type: string;
        /**
         * The name property is available only when type is 'ECDH'.
         */
        name?: string;
        /**
         * The size of parameter of an ephemeral key exchange.
         */
        size: number;
    }

    interface KeyObject {
        /**
         * Private keys in PEM format.
         */
        pem: string | Buffer;
        /**
         * Optional passphrase.
         */
        passphrase?: string;
    }

    interface PxfObject {
        /**
         * PFX or PKCS12 encoded private key and certificate chain.
         */
        buf: string | Buffer;
        /**
         * Optional passphrase.
         */
        passphrase?: string;
    }

    interface TLSSocketOptions extends SecureContextOptions, CommonConnectionOptions {
        /**
         * If true the TLS socket will be instantiated in server-mode.
         * Defaults to false.
         */
        isServer?: boolean;
        /**
         * An optional net.Server instance.
         */
        server?: net.Server;

        /**
         * An optional Buffer instance containing a TLS session.
         */
        session?: Buffer;
        /**
         * If true, specifies that the OCSP status request extension will be
         * added to the client hello and an 'OCSPResponse' event will be
         * emitted on the socket before establishing a secure communication
         */
        requestOCSP?: boolean;
    }

    class TLSSocket extends net.Socket {
        /**
         * Construct a new tls.TLSSocket object from an existing TCP socket.
         */
        constructor(socket: net.Socket, options?: TLSSocketOptions);

        /**
         * A boolean that is true if the peer certificate was signed by one of the specified CAs, otherwise false.
         */
        authorized: boolean;
        /**
         * The reason why the peer's certificate has not been verified.
         * This property becomes available only when tlsSocket.authorized === false.
         */
        authorizationError: Error;
        /**
         * Static boolean value, always true.
         * May be used to distinguish TLS sockets from regular ones.
         */
        encrypted: boolean;

        /**
         * String containing the selected ALPN protocol.
         * When ALPN has no selected protocol, tlsSocket.alpnProtocol equals false.
         */
        alpnProtocol?: string;

        /**
         * Returns an object representing the local certificate. The returned
         * object has some properties corresponding to the fields of the
         * certificate.
         *
         * See tls.TLSSocket.getPeerCertificate() for an example of the
         * certificate structure.
         *
         * If there is no local certificate, an empty object will be returned.
         * If the socket has been destroyed, null will be returned.
         */
        getCertificate(): PeerCertificate | object | null;
        /**
         * Returns an object representing the cipher name and the SSL/TLS protocol version of the current connection.
         * @returns Returns an object representing the cipher name
         * and the SSL/TLS protocol version of the current connection.
         */
        getCipher(): CipherNameAndProtocol;
        /**
         * Returns an object representing the type, name, and size of parameter
         * of an ephemeral key exchange in Perfect Forward Secrecy on a client
         * connection. It returns an empty object when the key exchange is not
         * ephemeral. As this is only supported on a client socket; null is
         * returned if called on a server socket. The supported types are 'DH'
         * and 'ECDH'. The name property is available only when type is 'ECDH'.
         *
         * For example: { type: 'ECDH', name: 'prime256v1', size: 256 }.
         */
        getEphemeralKeyInfo(): EphemeralKeyInfo | object | null;
        /**
         * Returns the latest Finished message that has
         * been sent to the socket as part of a SSL/TLS handshake, or undefined
         * if no Finished message has been sent yet.
         *
         * As the Finished messages are message digests of the complete
         * handshake (with a total of 192 bits for TLS 1.0 and more for SSL
         * 3.0), they can be used for external authentication procedures when
         * the authentication provided by SSL/TLS is not desired or is not
         * enough.
         *
         * Corresponds to the SSL_get_finished routine in OpenSSL and may be
         * used to implement the tls-unique channel binding from RFC 5929.
         */
        getFinished(): Buffer | undefined;
        /**
         * Returns an object representing the peer's certificate.
         * The returned object has some properties corresponding to the field of the certificate.
         * If detailed argument is true the full chain with issuer property will be returned,
         * if false only the top certificate without issuer property.
         * If the peer does not provide a certificate, it returns null or an empty object.
         * @param detailed - If true; the full chain with issuer property will be returned.
         * @returns An object representing the peer's certificate.
         */
        getPeerCertificate(detailed: true): DetailedPeerCertificate;
        getPeerCertificate(detailed?: false): PeerCertificate;
        getPeerCertificate(detailed?: boolean): PeerCertificate | DetailedPeerCertificate;
        /**
         * Returns the latest Finished message that is expected or has actually
         * been received from the socket as part of a SSL/TLS handshake, or
         * undefined if there is no Finished message so far.
         *
         * As the Finished messages are message digests of the complete
         * handshake (with a total of 192 bits for TLS 1.0 and more for SSL
         * 3.0), they can be used for external authentication procedures when
         * the authentication provided by SSL/TLS is not desired or is not
         * enough.
         *
         * Corresponds to the SSL_get_peer_finished routine in OpenSSL and may
         * be used to implement the tls-unique channel binding from RFC 5929.
         */
        getPeerFinished(): Buffer | undefined;
        /**
         * Returns a string containing the negotiated SSL/TLS protocol version of the current connection.
         * The value `'unknown'` will be returned for connected sockets that have not completed the handshaking process.
         * The value `null` will be returned for server sockets or disconnected client sockets.
         * See https://www.openssl.org/docs/man1.0.2/ssl/SSL_get_version.html for more information.
         * @returns negotiated SSL/TLS protocol version of the current connection
         */
        getProtocol(): string | null;
        /**
         * Could be used to speed up handshake establishment when reconnecting to the server.
         * @returns ASN.1 encoded TLS session or undefined if none was negotiated.
         */
        getSession(): Buffer | undefined;
        /**
         * Returns a list of signature algorithms shared between the server and
         * the client in the order of decreasing preference.
         */
        getSharedSigalgs(): string[];
        /**
         * NOTE: Works only with client TLS sockets.
         * Useful only for debugging, for session reuse provide session option to tls.connect().
         * @returns TLS session ticket or undefined if none was negotiated.
         */
        getTLSTicket(): Buffer | undefined;
        /**
         * Returns true if the session was reused, false otherwise.
         */
        isSessionReused(): boolean;
        /**
         * Initiate TLS renegotiation process.
         *
         * NOTE: Can be used to request peer's certificate after the secure connection has been established.
         * ANOTHER NOTE: When running as the server, socket will be destroyed with an error after handshakeTimeout timeout.
         * @param options - The options may contain the following fields: rejectUnauthorized,
         * requestCert (See tls.createServer() for details).
         * @param callback - callback(err) will be executed with null as err, once the renegotiation
         * is successfully completed.
         * @return `undefined` when socket is destroy, `false` if negotiaion can't be initiated.
         */
        renegotiate(options: { rejectUnauthorized?: boolean, requestCert?: boolean }, callback: (err: Error | null) => void): undefined | boolean;
        /**
         * Set maximum TLS fragment size (default and maximum value is: 16384, minimum is: 512).
         * Smaller fragment size decreases buffering latency on the client: large fragments are buffered by
         * the TLS layer until the entire fragment is received and its integrity is verified;
         * large fragments can span multiple roundtrips, and their processing can be delayed due to packet
         * loss or reordering. However, smaller fragments add extra TLS framing bytes and CPU overhead,
         * which may decrease overall server throughput.
         * @param size - TLS fragment size (default and maximum value is: 16384, minimum is: 512).
         * @returns Returns true on success, false otherwise.
         */
        setMaxSendFragment(size: number): boolean;

        /**
         * Disables TLS renegotiation for this TLSSocket instance. Once called,
         * attempts to renegotiate will trigger an 'error' event on the
         * TLSSocket.
         */
        disableRenegotiation(): void;

        /**
         * When enabled, TLS packet trace information is written to `stderr`. This can be
         * used to debug TLS connection problems.
         *
         * Note: The format of the output is identical to the output of `openssl s_client
         * -trace` or `openssl s_server -trace`. While it is produced by OpenSSL's
         * `SSL_trace()` function, the format is undocumented, can change without notice,
         * and should not be relied on.
         */
        enableTrace(): void;

        addListener(event: string, listener: (...args: any[]) => void): this;
        addListener(event: "OCSPResponse", listener: (response: Buffer) => void): this;
        addListener(event: "secureConnect", listener: () => void): this;
        addListener(event: "session", listener: (session: Buffer) => void): this;
        addListener(event: "keylog", listener: (line: Buffer) => void): this;

        emit(event: string | symbol, ...args: any[]): boolean;
        emit(event: "OCSPResponse", response: Buffer): boolean;
        emit(event: "secureConnect"): boolean;
        emit(event: "session", session: Buffer): boolean;
        emit(event: "keylog", line: Buffer): boolean;

        on(event: string, listener: (...args: any[]) => void): this;
        on(event: "OCSPResponse", listener: (response: Buffer) => void): this;
        on(event: "secureConnect", listener: () => void): this;
        on(event: "session", listener: (session: Buffer) => void): this;
        on(event: "keylog", listener: (line: Buffer) => void): this;

        once(event: string, listener: (...args: any[]) => void): this;
        once(event: "OCSPResponse", listener: (response: Buffer) => void): this;
        once(event: "secureConnect", listener: () => void): this;
        once(event: "session", listener: (session: Buffer) => void): this;
        once(event: "keylog", listener: (line: Buffer) => void): this;

        prependListener(event: string, listener: (...args: any[]) => void): this;
        prependListener(event: "OCSPResponse", listener: (response: Buffer) => void): this;
        prependListener(event: "secureConnect", listener: () => void): this;
        prependListener(event: "session", listener: (session: Buffer) => void): this;
        prependListener(event: "keylog", listener: (line: Buffer) => void): this;

        prependOnceListener(event: string, listener: (...args: any[]) => void): this;
        prependOnceListener(event: "OCSPResponse", listener: (response: Buffer) => void): this;
        prependOnceListener(event: "secureConnect", listener: () => void): this;
        prependOnceListener(event: "session", listener: (session: Buffer) => void): this;
        prependOnceListener(event: "keylog", listener: (line: Buffer) => void): this;
    }

    interface CommonConnectionOptions {
        /**
         * An optional TLS context object from tls.createSecureContext()
         */
        secureContext?: SecureContext;

        /**
         * When enabled, TLS packet trace information is written to `stderr`. This can be
         * used to debug TLS connection problems.
         * @default false
         */
        enableTrace?: boolean;
        /**
         * If true the server will request a certificate from clients that
         * connect and attempt to verify that certificate. Defaults to
         * false.
         */
        requestCert?: boolean;
        /**
         * An array of strings or a Buffer naming possible ALPN protocols.
         * (Protocols should be ordered by their priority.)
         */
        ALPNProtocols?: string[] | Uint8Array[] | Uint8Array;
        /**
         * SNICallback(servername, cb) <Function> A function that will be
         * called if the client supports SNI TLS extension. Two arguments
         * will be passed when called: servername and cb. SNICallback should
         * invoke cb(null, ctx), where ctx is a SecureContext instance.
         * (tls.createSecureContext(...) can be used to get a proper
         * SecureContext.) If SNICallback wasn't provided the default callback
         * with high-level API will be used (see below).
         */
        SNICallback?: (servername: string, cb: (err: Error | null, ctx: SecureContext) => void) => void;
        /**
         * If true the server will reject any connection which is not
         * authorized with the list of supplied CAs. This option only has an
         * effect if requestCert is true.
         * @default true
         */
        rejectUnauthorized?: boolean;
    }

    interface TlsOptions extends SecureContextOptions, CommonConnectionOptions {
        /**
         * Abort the connection if the SSL/TLS handshake does not finish in the
         * specified number of milliseconds. A 'tlsClientError' is emitted on
         * the tls.Server object whenever a handshake times out. Default:
         * 120000 (120 seconds).
         */
        handshakeTimeout?: number;
        /**
         * The number of seconds after which a TLS session created by the
         * server will no longer be resumable. See Session Resumption for more
         * information. Default: 300.
         */
        sessionTimeout?: number;
        /**
         * 48-bytes of cryptographically strong pseudo-random data.
         */
        ticketKeys?: Buffer;

        /**
         *
         * @param socket
         * @param identity identity parameter sent from the client.
         * @return pre-shared key that must either be
         * a buffer or `null` to stop the negotiation process. Returned PSK must be
         * compatible with the selected cipher's digest.
         *
         * When negotiating TLS-PSK (pre-shared keys), this function is called
         * with the identity provided by the client.
         * If the return value is `null` the negotiation process will stop and an
         * "unknown_psk_identity" alert message will be sent to the other party.
         * If the server wishes to hide the fact that the PSK identity was not known,
         * the callback must provide some random data as `psk` to make the connection
         * fail with "decrypt_error" before negotiation is finished.
         * PSK ciphers are disabled by default, and using TLS-PSK thus
         * requires explicitly specifying a cipher suite with the `ciphers` option.
         * More information can be found in the RFC 4279.
         */

        pskCallback?(socket: TLSSocket, identity: string): DataView | NodeJS.TypedArray | null;
        /**
         * hint to send to a client to help
         * with selecting the identity during TLS-PSK negotiation. Will be ignored
         * in TLS 1.3. Upon failing to set pskIdentityHint `tlsClientError` will be
         * emitted with `ERR_TLS_PSK_SET_IDENTIY_HINT_FAILED` code.
         */
        pskIdentityHint?: string;
    }

    interface PSKCallbackNegotation {
        psk: DataView | NodeJS.TypedArray;
        identitty: string;
    }

    interface ConnectionOptions extends SecureContextOptions, CommonConnectionOptions {
        host?: string;
        port?: number;
        path?: string; // Creates unix socket connection to path. If this option is specified, `host` and `port` are ignored.
        socket?: net.Socket; // Establish secure connection on a given socket rather than creating a new socket
        checkServerIdentity?: typeof checkServerIdentity;
        servername?: string; // SNI TLS Extension
        session?: Buffer;
        minDHSize?: number;
        lookup?: net.LookupFunction;
        timeout?: number;
        /**
         * When negotiating TLS-PSK (pre-shared keys), this function is called
         * with optional identity `hint` provided by the server or `null`
         * in case of TLS 1.3 where `hint` was removed.
         * It will be necessary to provide a custom `tls.checkServerIdentity()`
         * for the connection as the default one will try to check hostname/IP
         * of the server against the certificate but that's not applicable for PSK
         * because there won't be a certificate present.
         * More information can be found in the RFC 4279.
         *
         * @param hint message sent from the server to help client
         * decide which identity to use during negotiation.
         * Always `null` if TLS 1.3 is used.
         * @returns Return `null` to stop the negotiation process. `psk` must be
         * compatible with the selected cipher's digest.
         * `identity` must use UTF-8 encoding.
         */
        pskCallback?(hint: string | null): PSKCallbackNegotation | null;
    }

    class Server extends net.Server {
        /**
         * The server.addContext() method adds a secure context that will be
         * used if the client request's SNI name matches the supplied hostname
         * (or wildcard).
         */
        addContext(hostName: string, credentials: SecureContextOptions): void;
        /**
         * Returns the session ticket keys.
         */
        getTicketKeys(): Buffer;
        /**
         *
         * The server.setSecureContext() method replaces the
         * secure context of an existing server. Existing connections to the
         * server are not interrupted.
         */
        setSecureContext(details: SecureContextOptions): void;
        /**
         * The server.setSecureContext() method replaces the secure context of
         * an existing server. Existing connections to the server are not
         * interrupted.
         */
        setTicketKeys(keys: Buffer): void;

        /**
         * events.EventEmitter
         * 1. tlsClientError
         * 2. newSession
         * 3. OCSPRequest
         * 4. resumeSession
         * 5. secureConnection
         * 6. keylog
         */
        addListener(event: string, listener: (...args: any[]) => void): this;
        addListener(event: "tlsClientError", listener: (err: Error, tlsSocket: TLSSocket) => void): this;
        addListener(event: "newSession", listener: (sessionId: Buffer, sessionData: Buffer, callback: (err: Error, resp: Buffer) => void) => void): this;
        addListener(event: "OCSPRequest", listener: (certificate: Buffer, issuer: Buffer, callback: (err: Error | null, resp: Buffer) => void) => void): this;
        addListener(event: "resumeSession", listener: (sessionId: Buffer, callback: (err: Error, sessionData: Buffer) => void) => void): this;
        addListener(event: "secureConnection", listener: (tlsSocket: TLSSocket) => void): this;
        addListener(event: "keylog", listener: (line: Buffer, tlsSocket: TLSSocket) => void): this;

        emit(event: string | symbol, ...args: any[]): boolean;
        emit(event: "tlsClientError", err: Error, tlsSocket: TLSSocket): boolean;
        emit(event: "newSession", sessionId: Buffer, sessionData: Buffer, callback: (err: Error, resp: Buffer) => void): boolean;
        emit(event: "OCSPRequest", certificate: Buffer, issuer: Buffer, callback: (err: Error | null, resp: Buffer) => void): boolean;
        emit(event: "resumeSession", sessionId: Buffer, callback: (err: Error, sessionData: Buffer) => void): boolean;
        emit(event: "secureConnection", tlsSocket: TLSSocket): boolean;
        emit(event: "keylog", line: Buffer, tlsSocket: TLSSocket): boolean;

        on(event: string, listener: (...args: any[]) => void): this;
        on(event: "tlsClientError", listener: (err: Error, tlsSocket: TLSSocket) => void): this;
        on(event: "newSession", listener: (sessionId: Buffer, sessionData: Buffer, callback: (err: Error, resp: Buffer) => void) => void): this;
        on(event: "OCSPRequest", listener: (certificate: Buffer, issuer: Buffer, callback: (err: Error | null, resp: Buffer) => void) => void): this;
        on(event: "resumeSession", listener: (sessionId: Buffer, callback: (err: Error, sessionData: Buffer) => void) => void): this;
        on(event: "secureConnection", listener: (tlsSocket: TLSSocket) => void): this;
        on(event: "keylog", listener: (line: Buffer, tlsSocket: TLSSocket) => void): this;

        once(event: string, listener: (...args: any[]) => void): this;
        once(event: "tlsClientError", listener: (err: Error, tlsSocket: TLSSocket) => void): this;
        once(event: "newSession", listener: (sessionId: Buffer, sessionData: Buffer, callback: (err: Error, resp: Buffer) => void) => void): this;
        once(event: "OCSPRequest", listener: (certificate: Buffer, issuer: Buffer, callback: (err: Error | null, resp: Buffer) => void) => void): this;
        once(event: "resumeSession", listener: (sessionId: Buffer, callback: (err: Error, sessionData: Buffer) => void) => void): this;
        once(event: "secureConnection", listener: (tlsSocket: TLSSocket) => void): this;
        once(event: "keylog", listener: (line: Buffer, tlsSocket: TLSSocket) => void): this;

        prependListener(event: string, listener: (...args: any[]) => void): this;
        prependListener(event: "tlsClientError", listener: (err: Error, tlsSocket: TLSSocket) => void): this;
        prependListener(event: "newSession", listener: (sessionId: Buffer, sessionData: Buffer, callback: (err: Error, resp: Buffer) => void) => void): this;
        prependListener(event: "OCSPRequest", listener: (certificate: Buffer, issuer: Buffer, callback: (err: Error | null, resp: Buffer) => void) => void): this;
        prependListener(event: "resumeSession", listener: (sessionId: Buffer, callback: (err: Error, sessionData: Buffer) => void) => void): this;
        prependListener(event: "secureConnection", listener: (tlsSocket: TLSSocket) => void): this;
        prependListener(event: "keylog", listener: (line: Buffer, tlsSocket: TLSSocket) => void): this;

        prependOnceListener(event: string, listener: (...args: any[]) => void): this;
        prependOnceListener(event: "tlsClientError", listener: (err: Error, tlsSocket: TLSSocket) => void): this;
        prependOnceListener(event: "newSession", listener: (sessionId: Buffer, sessionData: Buffer, callback: (err: Error, resp: Buffer) => void) => void): this;
        prependOnceListener(event: "OCSPRequest", listener: (certificate: Buffer, issuer: Buffer, callback: (err: Error | null, resp: Buffer) => void) => void): this;
        prependOnceListener(event: "resumeSession", listener: (sessionId: Buffer, callback: (err: Error, sessionData: Buffer) => void) => void): this;
        prependOnceListener(event: "secureConnection", listener: (tlsSocket: TLSSocket) => void): this;
        prependOnceListener(event: "keylog", listener: (line: Buffer, tlsSocket: TLSSocket) => void): this;
    }

    interface SecurePair {
        encrypted: TLSSocket;
        cleartext: TLSSocket;
    }

    type SecureVersion = 'TLSv1.3' | 'TLSv1.2' | 'TLSv1.1' | 'TLSv1';

    interface SecureContextOptions {
        /**
         * Optionally override the trusted CA certificates. Default is to trust
         * the well-known CAs curated by Mozilla. Mozilla's CAs are completely
         * replaced when CAs are explicitly specified using this option.
         */
        ca?: string | Buffer | Array<string | Buffer>;
        /**
         *  Cert chains in PEM format. One cert chain should be provided per
         *  private key. Each cert chain should consist of the PEM formatted
         *  certificate for a provided private key, followed by the PEM
         *  formatted intermediate certificates (if any), in order, and not
         *  including the root CA (the root CA must be pre-known to the peer,
         *  see ca). When providing multiple cert chains, they do not have to
         *  be in the same order as their private keys in key. If the
         *  intermediate certificates are not provided, the peer will not be
         *  able to validate the certificate, and the handshake will fail.
         */
        cert?: string | Buffer | Array<string | Buffer>;
        /**
         *  Colon-separated list of supported signature algorithms. The list
         *  can contain digest algorithms (SHA256, MD5 etc.), public key
         *  algorithms (RSA-PSS, ECDSA etc.), combination of both (e.g
         *  'RSA+SHA384') or TLS v1.3 scheme names (e.g. rsa_pss_pss_sha512).
         */
        sigalgs?: string;
        /**
         * Cipher suite specification, replacing the default. For more
         * information, see modifying the default cipher suite. Permitted
         * ciphers can be obtained via tls.getCiphers(). Cipher names must be
         * uppercased in order for OpenSSL to accept them.
         */
        ciphers?: string;
        /**
         * Name of an OpenSSL engine which can provide the client certificate.
         */
        clientCertEngine?: string;
        /**
         * PEM formatted CRLs (Certificate Revocation Lists).
         */
        crl?: string | Buffer | Array<string | Buffer>;
        /**
         * Diffie Hellman parameters, required for Perfect Forward Secrecy. Use
         * openssl dhparam to create the parameters. The key length must be
         * greater than or equal to 1024 bits or else an error will be thrown.
         * Although 1024 bits is permissible, use 2048 bits or larger for
         * stronger security. If omitted or invalid, the parameters are
         * silently discarded and DHE ciphers will not be available.
         */
        dhparam?: string | Buffer;
        /**
         * A string describing a named curve or a colon separated list of curve
         * NIDs or names, for example P-521:P-384:P-256, to use for ECDH key
         * agreement. Set to auto to select the curve automatically. Use
         * crypto.getCurves() to obtain a list of available curve names. On
         * recent releases, openssl ecparam -list_curves will also display the
         * name and description of each available elliptic curve. Default:
         * tls.DEFAULT_ECDH_CURVE.
         */
        ecdhCurve?: string;
        /**
         * Attempt to use the server's cipher suite preferences instead of the
         * client's. When true, causes SSL_OP_CIPHER_SERVER_PREFERENCE to be
         * set in secureOptions
         */
        honorCipherOrder?: boolean;
        /**
         * Private keys in PEM format. PEM allows the option of private keys
         * being encrypted. Encrypted keys will be decrypted with
         * options.passphrase. Multiple keys using different algorithms can be
         * provided either as an array of unencrypted key strings or buffers,
         * or an array of objects in the form {pem: <string|buffer>[,
         * passphrase: <string>]}. The object form can only occur in an array.
         * object.passphrase is optional. Encrypted keys will be decrypted with
         * object.passphrase if provided, or options.passphrase if it is not.
         */
        key?: string | Buffer | Array<Buffer | KeyObject>;
        /**
         * Name of an OpenSSL engine to get private key from. Should be used
         * together with privateKeyIdentifier.
         */
        privateKeyEngine?: string;
        /**
         * Identifier of a private key managed by an OpenSSL engine. Should be
         * used together with privateKeyEngine. Should not be set together with
         * key, because both options define a private key in different ways.
         */
        privateKeyIdentifier?: string;
        /**
         * Optionally set the maximum TLS version to allow. One
         * of `'TLSv1.3'`, `'TLSv1.2'`, `'TLSv1.1'`, or `'TLSv1'`. Cannot be specified along with the
         * `secureProtocol` option, use one or the other.
         * **Default:** `'TLSv1.3'`, unless changed using CLI options. Using
         * `--tls-max-v1.2` sets the default to `'TLSv1.2'`. Using `--tls-max-v1.3` sets the default to
         * `'TLSv1.3'`. If multiple of the options are provided, the highest maximum is used.
         */
        maxVersion?: SecureVersion;
        /**
         * Optionally set the minimum TLS version to allow. One
         * of `'TLSv1.3'`, `'TLSv1.2'`, `'TLSv1.1'`, or `'TLSv1'`. Cannot be specified along with the
         * `secureProtocol` option, use one or the other.  It is not recommended to use
         * less than TLSv1.2, but it may be required for interoperability.
         * **Default:** `'TLSv1.2'`, unless changed using CLI options. Using
         * `--tls-v1.0` sets the default to `'TLSv1'`. Using `--tls-v1.1` sets the default to
         * `'TLSv1.1'`. Using `--tls-min-v1.3` sets the default to
         * 'TLSv1.3'. If multiple of the options are provided, the lowest minimum is used.
         */
        minVersion?: SecureVersion;
        /**
         * Shared passphrase used for a single private key and/or a PFX.
         */
        passphrase?: string;
        /**
         * PFX or PKCS12 encoded private key and certificate chain. pfx is an
         * alternative to providing key and cert individually. PFX is usually
         * encrypted, if it is, passphrase will be used to decrypt it. Multiple
         * PFX can be provided either as an array of unencrypted PFX buffers,
         * or an array of objects in the form {buf: <string|buffer>[,
         * passphrase: <string>]}. The object form can only occur in an array.
         * object.passphrase is optional. Encrypted PFX will be decrypted with
         * object.passphrase if provided, or options.passphrase if it is not.
         */
        pfx?: string | Buffer | Array<string | Buffer | PxfObject>;
        /**
         * Optionally affect the OpenSSL protocol behavior, which is not
         * usually necessary. This should be used carefully if at all! Value is
         * a numeric bitmask of the SSL_OP_* options from OpenSSL Options
         */
        secureOptions?: number; // Value is a numeric bitmask of the `SSL_OP_*` options
        /**
         * Legacy mechanism to select the TLS protocol version to use, it does
         * not support independent control of the minimum and maximum version,
         * and does not support limiting the protocol to TLSv1.3. Use
         * minVersion and maxVersion instead. The possible values are listed as
         * SSL_METHODS, use the function names as strings. For example, use
         * 'TLSv1_1_method' to force TLS version 1.1, or 'TLS_method' to allow
         * any TLS protocol version up to TLSv1.3. It is not recommended to use
         * TLS versions less than 1.2, but it may be required for
         * interoperability. Default: none, see minVersion.
         */
        secureProtocol?: string;
        /**
         * Opaque identifier used by servers to ensure session state is not
         * shared between applications. Unused by clients.
         */
        sessionIdContext?: string;
    }

    interface SecureContext {
        context: any;
    }

    /*
     * Verifies the certificate `cert` is issued to host `host`.
     * @host The hostname to verify the certificate against
     * @cert PeerCertificate representing the peer's certificate
     *
     * Returns Error object, populating it with the reason, host and cert on failure.  On success, returns undefined.
     */
    function checkServerIdentity(host: string, cert: PeerCertificate): Error | undefined;
    function createServer(secureConnectionListener?: (socket: TLSSocket) => void): Server;
    function createServer(options: TlsOptions, secureConnectionListener?: (socket: TLSSocket) => void): Server;
    function connect(options: ConnectionOptions, secureConnectListener?: () => void): TLSSocket;
    function connect(port: number, host?: string, options?: ConnectionOptions, secureConnectListener?: () => void): TLSSocket;
    function connect(port: number, options?: ConnectionOptions, secureConnectListener?: () => void): TLSSocket;
    /**
     * @deprecated
     */
    function createSecurePair(credentials?: SecureContext, isServer?: boolean, requestCert?: boolean, rejectUnauthorized?: boolean): SecurePair;
    function createSecureContext(details: SecureContextOptions): SecureContext;
    function getCiphers(): string[];

    /**
     * The default curve name to use for ECDH key agreement in a tls server.
     * The default value is 'auto'. See tls.createSecureContext() for further
     * information.
     */
    let DEFAULT_ECDH_CURVE: string;
    /**
     * The default value of the maxVersion option of
     * tls.createSecureContext(). It can be assigned any of the supported TLS
     * protocol versions, 'TLSv1.3', 'TLSv1.2', 'TLSv1.1', or 'TLSv1'. Default:
     * 'TLSv1.3', unless changed using CLI options. Using --tls-max-v1.2 sets
     * the default to 'TLSv1.2'. Using --tls-max-v1.3 sets the default to
     * 'TLSv1.3'. If multiple of the options are provided, the highest maximum
     * is used.
     */
    let DEFAULT_MAX_VERSION: SecureVersion;
    /**
     * The default value of the minVersion option of tls.createSecureContext().
     * It can be assigned any of the supported TLS protocol versions,
     * 'TLSv1.3', 'TLSv1.2', 'TLSv1.1', or 'TLSv1'. Default: 'TLSv1.2', unless
     * changed using CLI options. Using --tls-min-v1.0 sets the default to
     * 'TLSv1'. Using --tls-min-v1.1 sets the default to 'TLSv1.1'. Using
     * --tls-min-v1.3 sets the default to 'TLSv1.3'. If multiple of the options
     * are provided, the lowest minimum is used.
     */
    let DEFAULT_MIN_VERSION: SecureVersion;

    /**
     * An immutable array of strings representing the root certificates (in PEM
     * format) used for verifying peer certificates. This is the default value
     * of the ca option to tls.createSecureContext().
     */
    const rootCertificates: ReadonlyArray<string>;
}
