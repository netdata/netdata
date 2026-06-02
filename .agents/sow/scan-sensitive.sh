#!/usr/bin/env bash
# Scan durable SOW-related artifacts for sensitive values.
# Usage: .agents/sow/scan-sensitive.sh FILE...

set -uo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: $0 FILE..." >&2
  exit 2
fi

failures=0

if ! command -v perl >/dev/null 2>&1; then
  echo "perl is required for sensitive-data scanning" >&2
  exit 2
fi

scan_sensitive_file() {
  local file="$1"
  perl -ne '
    chomp;
    my $line = $_;
    my @hits;

    sub is_public_customer_ip {
      my ($ip) = @_;
      my @o = split(/\./, $ip);
      return 0 unless @o == 4;
      return 0 if grep { $_ !~ /^\d+$/ || $_ < 0 || $_ > 255 } @o;
      return 0 if $o[0] == 10;
      return 0 if $o[0] == 172 && $o[1] >= 16 && $o[1] <= 31;
      return 0 if $o[0] == 192 && $o[1] == 168;
      return 0 if $o[0] == 127;
      return 0 if $o[0] == 169 && $o[1] == 254;
      return 0 if $o[0] == 100 && $o[1] >= 64 && $o[1] <= 127;
      return 0 if $o[0] == 0;
      return 0 if $o[0] >= 224;
      return 0 if $o[0] == 192 && $o[1] == 0 && $o[2] == 2;
      return 0 if $o[0] == 198 && $o[1] == 51 && $o[2] == 100;
      return 0 if $o[0] == 203 && $o[1] == 0 && $o[2] == 113;
      return 1;
    }

    push @hits, "private-key-material" if $line =~ /-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----/;
    push @hits, "aws-access-key" if $line =~ /\b(?:AKIA|ASIA)[0-9A-Z]{16}\b/;
    push @hits, "github-token" if $line =~ /\b(?:github_pat_[A-Za-z0-9_]{20,}|gh[pousr]_[A-Za-z0-9_]{20,})\b/;
    push @hits, "slack-token" if $line =~ /\bxox[baprs]-[A-Za-z0-9-]{20,}\b/;
    push @hits, "openai-key" if $line =~ /\bsk-(?:proj-)?[A-Za-z0-9_-]{20,}\b/;
    push @hits, "google-api-key" if $line =~ /\bAIza[0-9A-Za-z_-]{20,}\b/;
    push @hits, "jwt" if $line =~ /\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\b/;
    push @hits, "credentialed-url" if $line =~ m{[a-z][a-z0-9+.-]*://[^/\s:@]+:[^/\s:@]+@}i && $line !~ m{x-access-token:(?:TOK|TOKEN|REDACTED|EXAMPLE|PLACEHOLDER|YOUR[_-]?TOKEN)\@github\.com}i;
    push @hits, "bearer-token" if $line =~ /\bBearer\s+[A-Za-z0-9._~+\/=-]{16,}\b/i && $line !~ /\b(REDACTED|EXAMPLE|PLACEHOLDER|YOUR[_-]?(?:TOKEN|ACCESS[_-]?TOKEN|BEARER[_-]?TOKEN))\b/i;

    if ($line =~ /\b(?:pass(?:word)?|passwd|pwd|api[_-]?key|secret|token|client[_-]?secret|private[_-]?key|access[_-]?key)\b\s*[:=]\s*["'\''`]?([^"'\''`\s<>{}\[\]&,]{8,})/i) {
      my $value = lc $1;
      push @hits, "credential-assignment" unless $value =~ /^(redacted|example|placeholder|changeme|change-me|xxx|xxxx|null|none|your[_-]?|dummy|sample|fake|test)/ || $value =~ /^\$/ || $value =~ /^(config|settings|options|opts|env|process\.env|os\.environ)\./ || $value =~ /^[a-z_][a-z0-9_.]*(token|secret|key|password)[a-z0-9_.]*$/ || $value =~ /^(tok|token)\@github\.com\b/;
    }

    if ($line =~ /\b(?:snmp[_-]?)?(?:community|community[_-]?string|rocommunity|rwcommunity)\b\s*[:=]\s*["'\''`]?([^"'\''`\s<>{}\[\]]{3,})/i) {
      my $value = lc $1;
      push @hits, "snmp-community" unless $value =~ /^(redacted|example|placeholder|changeme|change-me|xxx|xxxx|null|none)$/;
    }

    if ($line =~ /\b(?:customer|client|tenant|account|organization|org|community[ _-]?member)[ _-](?:name|id|identifier)\b\s*[:=]\s*["'\''`]?([^"'\''`<>\[\]{}][^"'\''`<>\[\]{}]{2,})/i) {
      my $value = $1;
      $value =~ s/^\s+|\s+$//g;
      push @hits, "customer-or-private-identifier" unless $value =~ /^(redacted|example|placeholder|customer-|client-|tenant-|account-|org-|user|none|null)/i;
    }

    if ($line =~ /\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b/i) {
      push @hits, "email-address" unless $line =~ /\b(example\.com|example\.org|example\.net|localhost)\b/i || $line =~ /\bgit\@github\.com[:\/]/i || $line =~ /x-access-token:(?:TOK|TOKEN|REDACTED|EXAMPLE|PLACEHOLDER|YOUR[_-]?TOKEN)\@github\.com/i;
    }

    if ($line =~ /\b(customer|client|tenant|account|community member|support|production|prod|log|trace|request|source ip|remote ip|x-forwarded-for|host ip)\b/i) {
      while ($line =~ /\b((?:\d{1,3}\.){3}\d{1,3})\b/g) {
        push @hits, "public-ip-address" if is_public_customer_ip($1);
      }
    }

    for my $hit (@hits) {
      print "$ARGV:$.:$hit\n";
    }
  ' "$file"
}

for file in "$@"; do
  if [ ! -f "$file" ]; then
    echo "missing file: $file" >&2
    failures=$((failures + 1))
    continue
  fi

  if ! hits=$(scan_sensitive_file "$file"); then
    echo "failed to scan file: $file" >&2
    failures=$((failures + 1))
    continue
  fi

  if [ -n "$hits" ]; then
    printf '%s\n' "$hits"
    failures=$((failures + 1))
  fi
done

exit "$failures"
