//! Credential-safe rendering of remote-storage error text.
//!
//! [`redact`] is the crate's single text-level redaction pass: a pure string
//! transform, aware of no error type. Remote-storage errors embed full
//! request URLs. AWS-style requests carry credentials in the query string:
//! the raw web-identity JWT on the STS `AssumeRoleWithWebIdentity` call, and
//! `X-Amz-Signature` / `X-Amz-Security-Token` on query-signed requests. So no
//! query string can be trusted in a journal. [`redact`] drops query
//! strings wholesale, covering future credential parameter names by
//! construction at the cost of hiding benign query params; the scheme, host,
//! and path stay visible for diagnosis.

/// Replace the query string of every URL in `text` with `[REDACTED]`,
/// keeping the `?`: `https://host/path?a=b` becomes
/// `https://host/path?[REDACTED]`.
///
/// A `?` starts a query string only when the same whitespace-delimited token
/// already contained `://` (prose like "failed?" and bare paths are
/// untouched). The query is dropped up to the token's end or up to a
/// terminator. The terminator is kept, so surrounding punctuation from
/// formats like reqwest's `… for url (https://…)` survives. Terminators are
/// whitespace and `"`, `'`, `)`, `]`, `>`, `,`. The scan re-arms at the next
/// `://`, so every URL in the text is redacted.
///
/// Scope assumptions, fine for the error text reqwest and opendal produce: a
/// URL must carry an explicit `scheme://` (a bare `host:port/?…` passes
/// through). A URL split across error-chain levels (whitespace between `://`
/// and `?`) is not caught.
pub(crate) fn redact(text: &str) -> String {
    let chars: Vec<char> = text.chars().collect();
    let mut out = String::with_capacity(text.len());
    // Per-token state: marks the current whitespace-delimited token as
    // carrying a URL — armed by the `://` check below, cleared at whitespace
    // and by a completed redaction, so nothing outside the token is redacted.
    let mut in_url = false;
    let mut i = 0;
    while i < chars.len() {
        let c = chars[i];
        if c.is_whitespace() {
            in_url = false;
            out.push(c);
        } else if c == '?' && in_url {
            out.push_str("?[REDACTED]");
            while i + 1 < chars.len() {
                let n = chars[i + 1];
                if n.is_whitespace() || matches!(n, '"' | '\'' | ')' | ']' | '>' | ',') {
                    break;
                }
                i += 1;
            }
            in_url = false;
        } else {
            // The second slash of `://`.
            if c == '/' && i >= 2 && chars[i - 1] == '/' && chars[i - 2] == ':' {
                in_url = true;
            }
            out.push(c);
        }
        i += 1;
    }
    out
}

#[cfg(test)]
mod tests {
    use super::redact;

    #[test]
    fn strips_sts_web_identity_token() {
        let input = "request to https://sts.us-east-1.amazonaws.com/?Action=AssumeRoleWithWebIdentity&WebIdentityToken=SENTINEL_JWT&Version=2011-06-15 failed";
        let out = redact(input);
        assert!(!out.contains("SENTINEL_JWT"));
        assert_eq!(
            out,
            "request to https://sts.us-east-1.amazonaws.com/?[REDACTED] failed"
        );
    }

    #[test]
    fn redacts_the_field_reported_opendal_error_shape() {
        // Mirrors the exact structure of the field-reported leak: opendal's
        // Display (kind/status/op + context map + message) with a reqsign STS
        // failure as the source — RoleArn and the web-identity JWT both live
        // in the URL query. Values are sanitized stand-ins.
        let input = "Unexpected (temporary) at list, context: { called: reqsign::LoadCredential, service: s3, path: v1/tenants/default/sfst/2026-05-22/, listed: 0 } => loading credential to sign http request, source: error sending request for url (https://sts.us-east-1.amazonaws.com/?Action=AssumeRoleWithWebIdentity&RoleArn=arn:aws:iam::000000000000:role/EXAMPLE_ROLE&WebIdentityToken=eyJSENTINEL_HEADER.eyJSENTINEL_PAYLOAD.SENTINEL-SIG_with-mixed_chars123&Version=2011-06-15&RoleSessionName=reqsign)";
        let out = redact(input);
        assert!(!out.contains("SENTINEL"), "token leaked: {out}");
        assert!(!out.contains("EXAMPLE_ROLE"), "role arn leaked: {out}");
        assert_eq!(
            out,
            "Unexpected (temporary) at list, context: { called: reqsign::LoadCredential, service: s3, path: v1/tenants/default/sfst/2026-05-22/, listed: 0 } => loading credential to sign http request, source: error sending request for url (https://sts.us-east-1.amazonaws.com/?[REDACTED])"
        );
    }

    #[test]
    fn preserves_reqwest_parenthesized_url_format() {
        let input =
            "error sending request for url (https://sts.amazonaws.com/?WebIdentityToken=SENTINEL)";
        let out = redact(input);
        assert!(!out.contains("SENTINEL"));
        assert_eq!(
            out,
            "error sending request for url (https://sts.amazonaws.com/?[REDACTED])"
        );
    }

    #[test]
    fn redacts_every_url_in_the_text() {
        let input = "a https://x.example/?k=S1 then b https://y.example/p?t=S2, done";
        let out = redact(input);
        assert!(!out.contains("S1") && !out.contains("S2"));
        assert_eq!(
            out,
            "a https://x.example/?[REDACTED] then b https://y.example/p?[REDACTED], done"
        );
    }

    #[test]
    fn leaves_prose_question_marks_alone() {
        let input = "did the upload fail? maybe. path=/a/b?not-a-url anyway";
        assert_eq!(redact(input), input);
    }

    #[test]
    fn url_without_query_is_unchanged() {
        let input = "stat https://bucket.s3.amazonaws.com/key failed: not found";
        assert_eq!(redact(input), input);
    }
}
