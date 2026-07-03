//! Credential-safe rendering of remote-storage error text.
//!
//! Remote-storage errors can embed full request URLs, and AWS-style requests
//! carry credentials in the query string: the raw web-identity JWT on the STS
//! `AssumeRoleWithWebIdentity` call, `X-Amz-Signature` /
//! `X-Amz-Security-Token` on query-signed requests. Every log line built from
//! a storage error MUST pass through [`redact`], which drops URL query
//! strings wholesale — future credential parameter names are covered by
//! construction, at the cost of hiding benign query params. The scheme, host,
//! and path stay visible for diagnosis.

/// Replace the query string of every URL in `text` with `[REDACTED]`.
///
/// A `?` starts a query string only when the same whitespace-delimited token
/// previously contained `://` (prose like "failed?" is untouched). The query
/// is dropped up to the token's end or a closing delimiter, preserving
/// surrounding punctuation from formats like reqwest's `… for url (https://…)`.
pub(crate) fn redact(text: &str) -> String {
    let chars: Vec<char> = text.chars().collect();
    let mut out = String::with_capacity(text.len());
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
