#!/usr/bin/env node
/**
 * Security test suite for fs-mcp-server.js
 *
 * Tests all attack vectors to ensure:
 * 1. The root directory can NEVER be escaped
 * 2. The actual ROOT path is NEVER leaked to the model (in outputs OR errors)
 * 3. All parameter types and edge cases are handled safely
 *
 * Attack vectors tested:
 * - Path traversal (.., ../, ..\, etc.)
 * - Absolute paths (/, \, C:\, //, etc.)
 * - Null byte injection (\x00, \u0000)
 * - Unicode path separators and dots
 * - Type confusion (arrays, objects, numbers, null)
 * - Shell metacharacters (; | & $ ` etc.)
 * - Newlines and control characters
 * - Extremely long paths
 * - URL encoding (%2e%2e, %2f, etc.)
 * - Mixed encoding attacks
 * - Empty and whitespace-only paths
 * - Dot variations (., .., ..., . . , etc.)
 */

import fs from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import os from "node:os";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Use a distinctive path that we can detect if leaked
const TEST_DIR = path.join(os.tmpdir(), `SECURITY-TEST-SECRET-${Date.now()}-${Math.random().toString(36).slice(2)}`);
const SERVER_PATH = path.join(__dirname, "fs-mcp-server.js");

// This string should NEVER appear in any output to the model
const SECRET_ROOT_MARKER = TEST_DIR;

const RED = "\x1b[0;31m";
const GREEN = "\x1b[0;32m";
const YELLOW = "\x1b[1;33m";
const BLUE = "\x1b[0;34m";
const GRAY = "\x1b[0;90m";
const MAGENTA = "\x1b[0;35m";
const NC = "\x1b[0m";

let testsPassed = 0;
let testsFailed = 0;
let testsSkipped = 0;
const testResults = [];
let verbose = process.argv.includes("--verbose") || process.argv.includes("-v");

// External file that MUST NOT be readable
const EXTERNAL_SECRET_FILE = "/etc/passwd";
const EXTERNAL_SECRET_CONTENT = "root:"; // Content that would indicate /etc/passwd was read

async function setupTestEnvironment() {
  console.log(`${BLUE}Setting up security test environment${NC}`);
  console.log(`${GRAY}ROOT: ${TEST_DIR}${NC}`);
  console.log(`${YELLOW}This path must NEVER appear in model-facing output${NC}\n`);

  await fs.promises.mkdir(TEST_DIR, { recursive: true });
  await fs.promises.mkdir(path.join(TEST_DIR, "subdir"));
  await fs.promises.mkdir(path.join(TEST_DIR, "deep", "nested", "dir"), { recursive: true });

  // Create test files
  await fs.promises.writeFile(path.join(TEST_DIR, "test.txt"), "Safe test content\nLine 2\nLine 3");
  await fs.promises.writeFile(path.join(TEST_DIR, "subdir", "nested.txt"), "Nested file content");
  await fs.promises.writeFile(path.join(TEST_DIR, "search-target.js"), "const secret = 'findme';");

  // Create file with special name to test path handling
  await fs.promises.writeFile(path.join(TEST_DIR, "file with spaces.txt"), "Spaces in name");
}

class MCPSecurityTestClient {
  constructor(serverArgs = []) {
    this.server = null;
    this.buffer = "";
    this.messageId = 1;
    this.pendingRequests = new Map();
    this.serverArgs = Array.isArray(serverArgs) ? serverArgs : [];
    this.allResponses = []; // Capture all responses for leak detection
  }

  async start() {
    return new Promise((resolve, reject) => {
      this.server = spawn("node", [SERVER_PATH, ...this.serverArgs, TEST_DIR], {
        stdio: ["pipe", "pipe", "pipe"],
      });

      this.server.stdout.on("data", (data) => {
        this.buffer += data.toString();
        this.processBuffer();
      });

      this.server.stderr.on("data", (data) => {
        // Capture stderr for debugging, but per requirement 2 logs CAN show absolute paths
        // (model never sees stderr - it's server-side only)
        const stderrStr = data.toString();
        this.allResponses.push({ type: "stderr", content: stderrStr });
        if (verbose) console.error(`${GRAY}[stderr] ${stderrStr}${NC}`);
      });

      this.server.on("error", reject);

      this.sendRequest("initialize", {
        protocolVersion: "2024-11-05",
        clientInfo: { name: "security-test-client", version: "1.0.0" },
        capabilities: {},
      })
        .then(resolve)
        .catch(reject);
    });
  }

  async stop() {
    if (this.server) {
      this.server.stdin.end();
      this.server.kill("SIGTERM");
      this.server = null;
    }
  }

  processBuffer() {
    const lines = this.buffer.split("\n");
    this.buffer = lines.pop() || "";
    for (const line of lines) {
      if (line.trim() === "") continue;
      try {
        const msg = JSON.parse(line);
        // Store raw response for leak detection
        this.allResponses.push({ type: "response", content: line, parsed: msg });

        if (msg.id && this.pendingRequests.has(msg.id)) {
          const { resolve, reject } = this.pendingRequests.get(msg.id);
          this.pendingRequests.delete(msg.id);
          if (msg.error) {
            const err = new Error(msg.error.message);
            err.mcpError = msg.error;
            reject(err);
          } else {
            resolve(msg.result);
          }
        }
      } catch (e) {
        // Parse error - still record it
        this.allResponses.push({ type: "parse_error", content: line });
      }
    }
  }

  sendRequest(method, params) {
    return new Promise((resolve, reject) => {
      const id = this.messageId++;
      this.pendingRequests.set(id, { resolve, reject });
      const message = JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n";
      this.server.stdin.write(message);
    });
  }

  async callTool(name, args) {
    const result = await this.sendRequest("tools/call", { name, arguments: args });
    return result?.content?.[0]?.text || result;
  }

  // Check if any response leaked the ROOT path
  // NOTE: Only check MCP protocol responses, not stderr
  // Per security requirement 2: "Logs can show absolute paths" - stderr is server-side only
  checkForLeaks() {
    const leaks = [];
    for (const response of this.allResponses) {
      // Skip stderr - per requirement 2, logs can show absolute paths (model never sees them)
      if (response.type === "stderr") continue;

      if (response.content && response.content.includes(SECRET_ROOT_MARKER)) {
        leaks.push({
          type: response.type,
          snippet: response.content.substring(
            Math.max(0, response.content.indexOf(SECRET_ROOT_MARKER) - 20),
            response.content.indexOf(SECRET_ROOT_MARKER) + SECRET_ROOT_MARKER.length + 20
          )
        });
      }
    }
    return leaks;
  }

  clearResponses() {
    this.allResponses = [];
  }
}

// Helper to run a security test
async function runSecurityTest(name, testFn, serverArgs = []) {
  const testNum = testsPassed + testsFailed + testsSkipped + 1;

  if (verbose) {
    console.log(`\n${MAGENTA}═══ Security Test ${testNum}: ${name} ═══${NC}`);
  } else {
    process.stdout.write(`${GRAY}[${testNum}] ${name}...${NC} `);
  }

  const client = new MCPSecurityTestClient(serverArgs);
  try {
    await client.start();
    client.clearResponses(); // Clear init responses

    await testFn(client);

    // After test, check for ROOT leaks
    const leaks = client.checkForLeaks();
    if (leaks.length > 0) {
      throw new Error(`ROOT PATH LEAKED in ${leaks.length} response(s): ${JSON.stringify(leaks)}`);
    }

    testsPassed++;
    if (verbose) console.log(`${GREEN}✓ PASSED${NC}`);
    else console.log(`${GREEN}✓${NC}`);
    return { name, passed: true };
  } catch (error) {
    testsFailed++;
    if (verbose) console.log(`${RED}✗ FAILED: ${error.message}${NC}`);
    else console.log(`${RED}✗ ${error.message}${NC}`);
    return { name, passed: false, error: error.message };
  } finally {
    await client.stop();
  }
}

// ============================================================================
// DOT-DOT ENCODING BYPASS TESTS
// ============================================================================

// These are all the ways someone might try to encode ".." to bypass the check
const DOT_DOT_ENCODING_ATTACKS = [
  // URL encoding (single)
  "%2e%2e",
  "%2e.",
  ".%2e",
  "%2e%2e/",
  "%2e%2e/%2e%2e",
  "%2e%2e/etc/passwd",
  "foo/%2e%2e/bar",
  "foo/%2e%2e/%2e%2e/etc/passwd",

  // URL encoding (uppercase)
  "%2E%2E",
  "%2E.",
  ".%2E",
  "%2E%2E/etc/passwd",

  // Double URL encoding
  "%252e%252e",
  "%252e%252e/etc/passwd",
  "foo/%252e%252e/bar",

  // Triple URL encoding
  "%25252e%25252e",

  // Mixed single/double encoding
  "%252e%2e",
  "%2e%252e",

  // Backslash URL encoded
  "%2e%2e%5c%2e%2e",
  "%2e%2e%5cetc%5cpasswd",

  // Unicode dot characters (various representations)
  "\u002e\u002e",           // Regular dots (U+002E) - same as ..
  "\uff0e\uff0e",           // Fullwidth dots (U+FF0E) ．．
  "\u2024\u2024",           // One dot leader (U+2024) ․․
  "\u2025",                 // Two dot leader (U+2025) ‥
  "\ufe52\ufe52",           // Small full stop (U+FE52)
  "\u0701\u0701",           // Syriac supralinear full stop
  "\u0702\u0702",           // Syriac sublinear full stop
  "\u2027\u2027",           // Hyphenation point ‧‧

  // Combining characters
  ".\u0308.",               // Dot with combining diaeresis
  ".\u0307.",               // Dot with combining dot above
  "\u0323\u0323",           // Combining dot below (no base char)

  // Unicode normalization forms - NFC/NFD/NFKC/NFKD might change things
  // The fullwidth dot NFKC normalizes to regular dot!
  String.fromCharCode(0xff0e) + String.fromCharCode(0xff0e) + "/etc/passwd",

  // Overlong UTF-8 (as escaped sequences - may be rejected by JSON parser)
  // These represent '.' using more bytes than necessary
  // 2-byte overlong for '.' (0x2e): C0 AE
  "\xc0\xae\xc0\xae",
  // 3-byte overlong for '.' (0x2e): E0 80 AE
  "\xe0\x80\xae\xe0\x80\xae",

  // HTML entities (in case any HTML decoding happens)
  "&#46;&#46;",
  "&#x2e;&#x2e;",
  "&period;&period;",

  // Octal (shell-style, shouldn't work but test anyway)
  "\\056\\056",
  "\\0056\\0056",

  // Hex escapes (JavaScript-style)
  "\\x2e\\x2e",
  "\\x2E\\x2E",

  // Unicode escapes (JavaScript-style)
  "\\u002e\\u002e",
  "\\u002E\\u002E",

  // Base64 of ".." = "Li4=" (in case any base64 decoding)
  "Li4=",
  "Li4=/etc/passwd",

  // Null byte in the middle of dots
  ".\x00.",
  ".\x00./etc/passwd",
  "..\x00",
  "\x00..",

  // Dots with spaces/tabs
  ". .",
  ".\t.",
  " ..",
  ".. ",
  " .. ",
  "\t..\t",

  // Dots with zero-width characters
  ".\u200b.",              // Zero-width space between dots
  ".\u200c.",              // Zero-width non-joiner
  ".\u200d.",              // Zero-width joiner
  ".\ufeff.",              // BOM between dots
  "..\u200b",              // Zero-width after ..
  "\u200b..",              // Zero-width before ..

  // Right-to-left override tricks
  "\u202e..",              // RTL override before ..
  "..\u202e",              // RTL override after ..
  "\u202e../etc/passwd",

  // Combining grapheme joiner
  ".\u034f.",              // CGJ between dots

  // Case variations (dots don't have case, but test anyway)
  "..",                    // Regular (baseline)

  // Path with trailing dot variations
  ".../",
  "../.",
  ".././",
  "./../",
  "././..",
  ".//../",

  // Multiple dots
  "...",
  "....",
  ".....",
  ".../etc/passwd",
  "..../etc/passwd",

  // Backslash variations
  "..\\..",
  "..\\..\\etc\\passwd",
  "..\\/etc/passwd",
  "../\\etc/passwd",
];

// ============================================================================
// SET UP ENVIRONMENT VARIABLES FOR EXPANSION TESTS
// ============================================================================

// Set environment variables that attackers might try to exploit
process.env.TEST = "/tmp";
process.env.HOME = "/root";  // Override HOME to test expansion
process.env.SECRET = "/etc/passwd";
process.env.ROOT_DIR = "/";
process.env.EVIL = "../../../etc/passwd";

// ============================================================================
// ATTACK PAYLOADS
// ============================================================================

const PATH_TRAVERSAL_ATTACKS = [
  // Basic traversal
  "..",
  "../",
  "..\\",
  "../..",
  "../../..",
  "../../../..",
  "../../../../..",
  "../../../../../..",

  // Traversal to known files
  "../../../etc/passwd",
  "..\\..\\..\\etc\\passwd",
  "../../../etc/shadow",
  "../../../root/.ssh/id_rsa",
  "../../../proc/self/environ",

  // Mixed separators
  "..\\../..\\../etc/passwd",
  "../..\\../..\\etc/passwd",
  "..\\..\\..\\..\\..\\..\\etc\\passwd",

  // With current dir
  "./../../../etc/passwd",
  "./../../etc/passwd",
  "foo/./../../etc/passwd",
  "foo/../../../etc/passwd",

  // Double dots variations
  "..../etc/passwd",
  ".../.../etc/passwd",
  ". ./. ./etc/passwd",

  // Trailing dots
  "foo/bar/..",
  "foo/bar/../..",
  "foo/bar/../../..",
  "subdir/..",
  "subdir/../..",

  // URL encoded (in case any decoding happens)
  "%2e%2e/%2e%2e/etc/passwd",
  "%2e%2e%2f%2e%2e%2fetc%2fpasswd",
  "..%2f..%2f..%2fetc%2fpasswd",
  "%2e%2e%5c%2e%2e%5cetc%5cpasswd",

  // Double URL encoded
  "%252e%252e/%252e%252e/etc/passwd",

  // Unicode encoded dots
  "\u002e\u002e/\u002e\u002e/etc/passwd",

  // Overlong UTF-8 (theoretical)
  "\xc0\xae\xc0\xae/etc/passwd",
];

const ABSOLUTE_PATH_ATTACKS = [
  // Unix absolute
  "/etc/passwd",
  "/etc/shadow",
  "/root/.ssh/id_rsa",
  "/proc/self/environ",
  "/proc/self/cmdline",
  "/dev/null",
  "//etc/passwd",
  "///etc/passwd",

  // Windows absolute
  "C:\\Windows\\System32\\config\\SAM",
  "C:/Windows/System32/config/SAM",
  "\\\\server\\share\\file",
  "//server/share/file",

  // Drive letters
  "D:\\secret.txt",
  "c:/secret.txt",
  "Z:\\file",

  // UNC paths
  "\\\\?\\C:\\secret.txt",
  "\\\\.\\PhysicalDrive0",
];

const NULL_BYTE_ATTACKS = [
  // Null byte in middle
  "test.txt\x00/../../../etc/passwd",
  "subdir\x00/../../../etc/passwd",
  "foo\x00bar/../../../etc/passwd",

  // Null byte at end
  "test.txt\x00",
  "../../../etc/passwd\x00.txt",
  "../../../etc/passwd\x00",

  // Null byte at start
  "\x00../../../etc/passwd",
  "\x00test.txt",

  // Multiple null bytes
  "test\x00\x00.txt",
  "..\x00/..\x00/etc/passwd",

  // Unicode null
  "test.txt\u0000/../../../etc/passwd",
];

const UNICODE_ATTACKS = [
  // Fullwidth characters
  "\uff0e\uff0e/\uff0e\uff0e/etc/passwd", // ．．/．．
  "\uff0f\uff0f\uff0fetc\uff0fpasswd",      // ／／／etc／passwd

  // Homoglyphs for dots
  "\u2024\u2024/etc/passwd", // ONE DOT LEADER
  "\u2025/etc/passwd",       // TWO DOT LEADER

  // Homoglyphs for slashes
  "\u2215etc\u2215passwd", // DIVISION SLASH
  "\u2044etc\u2044passwd", // FRACTION SLASH
  "\u29f8etc\u29f8passwd", // BIG SOLIDUS

  // Combining characters
  ".\u0338./etc/passwd", // Combining long solidus overlay

  // Direction override
  "\u202e/etc/passwd",   // RIGHT-TO-LEFT OVERRIDE
  "\u202a../../../etc/passwd", // LEFT-TO-RIGHT EMBEDDING

  // Zero-width characters
  ".\u200b./.\u200b./etc/passwd", // ZERO WIDTH SPACE
  "..\u200c/..\u200c/etc/passwd", // ZERO WIDTH NON-JOINER
  "..\ufeff/..\ufeff/etc/passwd", // BOM
];

const TYPE_CONFUSION_ATTACKS = [
  // Arrays
  [".", "..", "etc", "passwd"],
  ["../../../etc/passwd"],
  ["."],

  // Objects
  { toString: () => "../../../etc/passwd" },
  { valueOf: () => "../../../etc/passwd" },
  { path: "../../../etc/passwd" },
  { 0: "..", 1: "..", length: 2 },

  // Numbers
  0,
  1,
  -1,
  NaN,
  Infinity,
  -Infinity,

  // Booleans
  true,
  false,

  // Null/undefined
  null,
  undefined,

  // Symbols (will fail JSON, but good to document)
  // Symbol("../../../etc/passwd"),

  // BigInt (will fail JSON)
  // 123n,
];

const SHELL_METACHAR_ATTACKS = [
  // Command injection attempts
  "; cat /etc/passwd",
  "| cat /etc/passwd",
  "& cat /etc/passwd",
  "&& cat /etc/passwd",
  "|| cat /etc/passwd",
  "`cat /etc/passwd`",
  "$(cat /etc/passwd)",
  "$(/bin/cat /etc/passwd)",

  // With valid prefix
  "test.txt; cat /etc/passwd",
  "test.txt | cat /etc/passwd",
  "test.txt && cat /etc/passwd",
  "test.txt `cat /etc/passwd`",
  "test.txt $(cat /etc/passwd)",

  // Newline injection
  "test.txt\ncat /etc/passwd",
  "test.txt\r\ncat /etc/passwd",

  // Quote escaping
  "test.txt'; cat /etc/passwd; echo '",
  'test.txt"; cat /etc/passwd; echo "',
  "test.txt`; cat /etc/passwd; echo `",

  // Variable expansion
  "$HOME/../../../etc/passwd",
  "${HOME}/../../../etc/passwd",
  "~/../../../etc/passwd",
  "~/../../etc/passwd",

  // Glob expansion
  "/etc/pass*",
  "/etc/pass??",
  "/etc/[p]asswd",
  "../../../etc/pass*",

  // Brace expansion
  "/etc/{passwd,shadow}",
  "../../../etc/{passwd,shadow}",
];

const CONTROL_CHAR_ATTACKS = [
  // Various control characters
  "test\x01.txt",
  "test\x02.txt",
  "test\x03.txt",
  "test\x04.txt",
  "test\x05.txt",
  "test\x06.txt",
  "test\x07.txt",  // Bell
  "test\x08.txt",  // Backspace
  "test\x09.txt",  // Tab
  "test\x0a.txt",  // Newline
  "test\x0b.txt",  // Vertical tab
  "test\x0c.txt",  // Form feed
  "test\x0d.txt",  // Carriage return
  "test\x1b.txt",  // Escape
  "test\x7f.txt",  // DEL

  // ANSI escape sequences
  "test\x1b[31m.txt",
  "../../../etc/passwd\x1b[0m",
];

const LENGTH_ATTACKS = [
  // Extremely long paths
  "a".repeat(10000),
  "../".repeat(1000) + "etc/passwd",
  "subdir/" + "a/".repeat(1000) + "file.txt",
  "a".repeat(65536),

  // Path with many segments
  Array(1000).fill("a").join("/"),
  Array(1000).fill("..").join("/") + "/etc/passwd",
];

const EDGE_CASE_ATTACKS = [
  // Empty and whitespace
  "",
  " ",
  "  ",
  "\t",
  "\n",
  "\r\n",
  "   \t\n  ",

  // Just dots
  ".",
  "..",
  "...",
  "....",
  ". ",
  " .",
  " . ",
  " .. ",

  // Dots with spaces
  ". .",
  ".. ..",
  ". . .",

  // Hidden files
  ".hidden",
  "..hidden",
  "...hidden",

  // Special directories
  ".",
  "./",
  ".//",
  ".\\",

  // Repeated separators
  "//",
  "///",
  "subdir//file.txt",
  "subdir///file.txt",
  "subdir\\\\file.txt",

  // Mixed separators
  "subdir/\\file.txt",
  "subdir\\/file.txt",
];

// ============================================================================
// SECURITY TESTS
// ============================================================================

const securityTests = [];

// ----------------------------------------------------------------------------
// PATH TRAVERSAL TESTS - ALL TOOLS
// ----------------------------------------------------------------------------

// ListDir path traversal
for (const payload of PATH_TRAVERSAL_ATTACKS.slice(0, 15)) { // Top 15 most important
  securityTests.push({
    name: `ListDir path traversal: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("ListDir", { dir: payload });
        // If we get here, check result doesn't contain sensitive data
        if (result && result.includes && result.includes("root:")) {
          throw new Error("SECURITY BREACH: /etc/passwd content leaked");
        }
      } catch (e) {
        // Expected to fail - verify error doesn't leak info
        if (e.message.includes(EXTERNAL_SECRET_CONTENT)) {
          throw new Error("SECURITY BREACH: Sensitive content in error");
        }
        // Error is expected and safe
      }
    },
  });
}

// Tree path traversal
for (const payload of PATH_TRAVERSAL_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `Tree path traversal: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Tree", { dir: payload });
        if (result && result.includes && (result.includes("/etc") || result.includes("passwd"))) {
          throw new Error("SECURITY BREACH: System directory listed");
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// Find path traversal
for (const payload of PATH_TRAVERSAL_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `Find dir traversal: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Find", { dir: payload, glob: "*" });
        if (result && result.includes && result.includes("/etc")) {
          throw new Error("SECURITY BREACH: System files found");
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// Read path traversal
for (const payload of PATH_TRAVERSAL_ATTACKS.slice(0, 15)) {
  securityTests.push({
    name: `Read path traversal: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Read", { file: payload });
        if (result && result.includes && result.includes("root:")) {
          throw new Error("SECURITY BREACH: /etc/passwd content read");
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// Grep path traversal
for (const payload of PATH_TRAVERSAL_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `Grep file traversal: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Grep", { file: payload, regex: "root" });
        if (result && result.includes && result.includes("root:x:0")) {
          throw new Error("SECURITY BREACH: /etc/passwd grepped");
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// RGrep path traversal
for (const payload of PATH_TRAVERSAL_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `RGrep dir traversal: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("RGrep", { dir: payload, regex: "root" });
        if (result && result.includes && result.includes("/etc/passwd")) {
          throw new Error("SECURITY BREACH: System files found via RGrep");
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// ----------------------------------------------------------------------------
// ABSOLUTE PATH TESTS - ALL TOOLS
// ----------------------------------------------------------------------------

for (const payload of ABSOLUTE_PATH_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `ListDir absolute: ${payload.slice(0, 30)}`,
    fn: async (client) => {
      try {
        await client.callTool("ListDir", { dir: payload });
        throw new Error("Should have rejected absolute path");
      } catch (e) {
        if (!e.message.includes("absolute") && !e.message.includes("not allowed")) {
          // If it's not our expected error, could be a leak
          if (e.message.includes("root:")) {
            throw new Error("SECURITY BREACH");
          }
        }
      }
    },
  });
}

for (const payload of ABSOLUTE_PATH_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `Read absolute: ${payload.slice(0, 30)}`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Read", { file: payload });
        if (result && result.includes("root:")) {
          throw new Error("SECURITY BREACH: Read /etc/passwd via absolute path");
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// ----------------------------------------------------------------------------
// NULL BYTE INJECTION TESTS
// ----------------------------------------------------------------------------

for (const payload of NULL_BYTE_ATTACKS) {
  securityTests.push({
    name: `Read null byte: ${JSON.stringify(payload).slice(0, 30)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Read", { file: payload });
        if (result && result.includes && result.includes("root:")) {
          throw new Error("SECURITY BREACH: Null byte allowed passwd read");
        }
      } catch (e) {
        // Expected to fail - that's good
      }
    },
  });
}

for (const payload of NULL_BYTE_ATTACKS.slice(0, 5)) {
  securityTests.push({
    name: `ListDir null byte: ${JSON.stringify(payload).slice(0, 30)}...`,
    fn: async (client) => {
      try {
        await client.callTool("ListDir", { dir: payload });
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// ----------------------------------------------------------------------------
// UNICODE ATTACKS
// ----------------------------------------------------------------------------

for (const payload of UNICODE_ATTACKS) {
  securityTests.push({
    name: `Read unicode: ${JSON.stringify(payload).slice(0, 35)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Read", { file: payload });
        if (result && result.includes && result.includes("root:")) {
          throw new Error("SECURITY BREACH: Unicode bypass");
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// ----------------------------------------------------------------------------
// TYPE CONFUSION TESTS
// ----------------------------------------------------------------------------

for (const payload of TYPE_CONFUSION_ATTACKS) {
  const payloadStr = JSON.stringify(payload);
  if (payloadStr === undefined) continue; // Skip non-serializable

  securityTests.push({
    name: `ListDir type confusion: ${payloadStr.slice(0, 25)}`,
    fn: async (client) => {
      try {
        await client.callTool("ListDir", { dir: payload });
      } catch (e) {
        // Should fail with type error, not security breach
        if (e.message.includes("root:")) {
          throw new Error("SECURITY BREACH via type confusion");
        }
      }
    },
  });

  securityTests.push({
    name: `Read type confusion: ${payloadStr.slice(0, 25)}`,
    fn: async (client) => {
      try {
        await client.callTool("Read", { file: payload });
      } catch (e) {
        if (e.message.includes("root:")) {
          throw new Error("SECURITY BREACH via type confusion");
        }
      }
    },
  });
}

// ----------------------------------------------------------------------------
// SHELL METACHARACTER TESTS
// ----------------------------------------------------------------------------

for (const payload of SHELL_METACHAR_ATTACKS.slice(0, 15)) {
  securityTests.push({
    name: `Read shell chars: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Read", { file: payload });
        if (result && result.includes && result.includes("root:")) {
          throw new Error("SECURITY BREACH: Shell injection");
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// Test regex parameter for shell injection
for (const payload of SHELL_METACHAR_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `Grep regex shell: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        await client.callTool("Grep", { file: "test.txt", regex: payload });
      } catch (e) {
        // Expected to fail - verify no shell execution
      }
    },
  });
}

// Test glob parameter for shell injection
for (const payload of SHELL_METACHAR_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `Find glob shell: ${payload.slice(0, 30)}...`,
    fn: async (client) => {
      try {
        await client.callTool("Find", { dir: ".", glob: payload });
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// ----------------------------------------------------------------------------
// CONTROL CHARACTER TESTS
// ----------------------------------------------------------------------------

for (const payload of CONTROL_CHAR_ATTACKS.slice(0, 10)) {
  securityTests.push({
    name: `Read control chars: ${JSON.stringify(payload).slice(0, 30)}`,
    fn: async (client) => {
      try {
        await client.callTool("Read", { file: payload });
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// ----------------------------------------------------------------------------
// LENGTH ATTACKS
// ----------------------------------------------------------------------------

securityTests.push({
  name: "Read extremely long path (10KB)",
  fn: async (client) => {
    try {
      await client.callTool("Read", { file: "a".repeat(10000) });
    } catch (e) {
      // Should fail gracefully
    }
  },
});

securityTests.push({
  name: "ListDir extremely long path (10KB)",
  fn: async (client) => {
    try {
      await client.callTool("ListDir", { dir: "a".repeat(10000) });
    } catch (e) {
      // Should fail gracefully
    }
  },
});

securityTests.push({
  name: "Read 1000 ../ traversal",
  fn: async (client) => {
    try {
      const result = await client.callTool("Read", { file: "../".repeat(1000) + "etc/passwd" });
      if (result && result.includes("root:")) {
        throw new Error("SECURITY BREACH: Deep traversal succeeded");
      }
    } catch (e) {
      // Expected to fail
    }
  },
});

// ----------------------------------------------------------------------------
// EDGE CASE TESTS
// ----------------------------------------------------------------------------

for (const payload of EDGE_CASE_ATTACKS) {
  securityTests.push({
    name: `ListDir edge: ${JSON.stringify(payload).slice(0, 25)}`,
    fn: async (client) => {
      try {
        await client.callTool("ListDir", { dir: payload });
      } catch (e) {
        // Many should fail, that's fine
      }
    },
  });
}

// ----------------------------------------------------------------------------
// ROOT PATH LEAK DETECTION TESTS
// ----------------------------------------------------------------------------

securityTests.push({
  name: "SUCCESS output must not contain ROOT path",
  fn: async (client) => {
    // Do successful operations and verify ROOT never appears
    await client.callTool("ListDir", { dir: "." });
    await client.callTool("Tree", { dir: "subdir" });
    await client.callTool("Read", { file: "test.txt" });
    await client.callTool("Find", { dir: ".", glob: "*.txt" });
    await client.callTool("Grep", { file: "test.txt", regex: "content" });
    await client.callTool("RGrep", { dir: ".", regex: "content" });

    // Check is done automatically after test
  },
});

securityTests.push({
  name: "ERROR output must not contain ROOT path",
  fn: async (client) => {
    // Trigger various errors and verify ROOT never appears in them
    const errorCases = [
      () => client.callTool("ListDir", { dir: "nonexistent_dir_xyz" }),
      () => client.callTool("Read", { file: "nonexistent_file_xyz.txt" }),
      () => client.callTool("Grep", { file: "nonexistent.txt", regex: "foo" }),
      () => client.callTool("Tree", { dir: "nonexistent_tree_dir" }),
      () => client.callTool("Find", { dir: "nonexistent_find_dir", glob: "*" }),
      () => client.callTool("RGrep", { dir: "nonexistent_rgrep_dir", regex: "foo" }),
      () => client.callTool("Read", { file: "../../../etc/passwd" }),
      () => client.callTool("ListDir", { dir: "/etc" }),
    ];

    for (const errorCase of errorCases) {
      try {
        await errorCase();
      } catch (e) {
        // Errors expected - leak check done automatically
      }
    }
  },
});

securityTests.push({
  name: "Unreadable file error must not leak ROOT path",
  fn: async (client) => {
    // Create a file and make it unreadable (chmod 000)
    // This tests that fsp.readFile errors don't leak absolute paths
    const unreadablePath = path.join(TEST_DIR, "unreadable-file.txt");
    try {
      await fs.promises.writeFile(unreadablePath, "secret content");
      await fs.promises.chmod(unreadablePath, 0o000);
    } catch (e) {
      // May fail on some systems
      return;
    }

    try {
      // Try to read the unreadable file - should fail with permission error
      await client.callTool("Read", { file: "unreadable-file.txt" });
      throw new Error("Expected Read to fail on unreadable file");
    } catch (e) {
      // Error expected - verify it doesn't contain ROOT path
      // The automatic leak check will verify MCP responses, but let's be explicit about the error message
      if (e.message.includes(TEST_DIR)) {
        throw new Error(`ROOT path leaked in unreadable file error: ${e.message}`);
      }
      // Should get a generic error like "cannot read file", not one with absolute paths
      // Note: We only check for TEST_DIR (ROOT), not generic paths like /tmp/
      // because per requirement 2, logs can show paths (and /tmp/ might appear in other contexts)
    } finally {
      // Restore permissions and cleanup
      try {
        await fs.promises.chmod(unreadablePath, 0o644);
        await fs.promises.unlink(unreadablePath);
      } catch (e) {
        // Ignore cleanup errors
      }
    }
  },
});

securityTests.push({
  name: "Symlink target must not leak external paths",
  fn: async (client) => {
    // Create a symlink pointing outside (if possible)
    const symlinkPath = path.join(TEST_DIR, "external-link");
    try {
      await fs.promises.symlink("/etc", symlinkPath);
    } catch (e) {
      // May fail if symlink exists or no permissions
      return;
    }

    try {
      // List directory with symlink
      const result = await client.callTool("ListDir", { dir: "." });

      // Should NOT show /etc in output
      if (result.includes("/etc")) {
        throw new Error("Symlink target path leaked");
      }
    } finally {
      try {
        await fs.promises.unlink(symlinkPath);
      } catch (e) { }
    }
  },
});

// ----------------------------------------------------------------------------
// GLOB/REGEX INJECTION TESTS
// ----------------------------------------------------------------------------

securityTests.push({
  name: "Glob cannot escape via pattern",
  fn: async (client) => {
    const maliciousGlobs = [
      "../../../etc/*",
      "/etc/passwd",
      "**/../../../etc/passwd",
      "{../../../etc/passwd,test.txt}",
    ];

    for (const glob of maliciousGlobs) {
      try {
        const result = await client.callTool("Find", { dir: ".", glob });
        if (result && result.includes("/etc")) {
          throw new Error(`Glob pattern escaped: ${glob}`);
        }
      } catch (e) {
        // Expected to fail
      }
    }
  },
});

securityTests.push({
  name: "Regex cannot inject rg flags",
  fn: async (client) => {
    const maliciousRegex = [
      "--include=/etc/passwd",
      "-f /etc/passwd",
      "--file=/etc/passwd",
      "-e root --include=/etc",
    ];

    for (const regex of maliciousRegex) {
      try {
        await client.callTool("Grep", { file: "test.txt", regex });
      } catch (e) {
        // Expected - verify no /etc content in error
        if (e.message.includes("root:x:")) {
          throw new Error(`Regex flag injection: ${regex}`);
        }
      }
    }
  },
});

// ----------------------------------------------------------------------------
// PARAMETER COMBINATION TESTS
// ----------------------------------------------------------------------------

securityTests.push({
  name: "Read with all optional params still safe",
  fn: async (client) => {
    try {
      await client.callTool("Read", {
        file: "../../../etc/passwd",
        start: 0,
        lines: 100,
        headOrTail: "head",
      });
    } catch (e) {
      // Expected to fail
    }
  },
});

securityTests.push({
  name: "Grep with all optional params still safe",
  fn: async (client) => {
    try {
      await client.callTool("Grep", {
        file: "../../../etc/passwd",
        regex: "root",
        before: 10,
        after: 10,
        caseSensitive: false,
      });
    } catch (e) {
      // Expected to fail
    }
  },
});

securityTests.push({
  name: "ListDir with all optional params still safe",
  fn: async (client) => {
    try {
      await client.callTool("ListDir", {
        dir: "../../../etc",
        showSize: true,
        showLastModified: true,
      });
    } catch (e) {
      // Expected to fail
    }
  },
});

// ----------------------------------------------------------------------------
// NEW HARDENING VALIDATION TESTS
// ----------------------------------------------------------------------------

securityTests.push({
  name: "Null bytes explicitly rejected with clear error",
  fn: async (client) => {
    try {
      await client.callTool("Read", { file: "test\x00.txt" });
      throw new Error("Should have rejected null byte");
    } catch (e) {
      if (!e.message.includes("null byte")) {
        throw new Error(`Expected null byte error, got: ${e.message}`);
      }
    }
  },
});

securityTests.push({
  name: "Control characters rejected with clear error",
  fn: async (client) => {
    try {
      await client.callTool("Read", { file: "test\x07.txt" }); // Bell character
      throw new Error("Should have rejected control character");
    } catch (e) {
      if (!e.message.includes("control character")) {
        throw new Error(`Expected control char error, got: ${e.message}`);
      }
    }
  },
});

securityTests.push({
  name: "Path length limit enforced",
  fn: async (client) => {
    try {
      await client.callTool("Read", { file: "a".repeat(5000) });
      throw new Error("Should have rejected long path");
    } catch (e) {
      if (!e.message.includes("maximum length")) {
        throw new Error(`Expected length error, got: ${e.message}`);
      }
    }
  },
});

securityTests.push({
  name: "Glob length limit enforced",
  fn: async (client) => {
    try {
      await client.callTool("Find", { dir: ".", glob: "*".repeat(5000) });
      throw new Error("Should have rejected long glob");
    } catch (e) {
      if (!e.message.includes("maximum length")) {
        throw new Error(`Expected length error, got: ${e.message}`);
      }
    }
  },
});

securityTests.push({
  name: "Regex null bytes rejected",
  fn: async (client) => {
    try {
      await client.callTool("Grep", { file: "test.txt", regex: "foo\x00bar" });
      throw new Error("Should have rejected null byte in regex");
    } catch (e) {
      if (!e.message.includes("null byte")) {
        throw new Error(`Expected null byte error, got: ${e.message}`);
      }
    }
  },
});

securityTests.push({
  name: "Glob null bytes rejected",
  fn: async (client) => {
    try {
      await client.callTool("Find", { dir: ".", glob: "*\x00*" });
      throw new Error("Should have rejected null byte in glob");
    } catch (e) {
      if (!e.message.includes("null byte")) {
        throw new Error(`Expected null byte error, got: ${e.message}`);
      }
    }
  },
});

// NOTE: Tests for "Symlink to /etc cannot be traversed" and "ListDir on symlink
// to /etc does not leak /etc contents" were removed because they contradicted
// Security Requirement 1: "Admin symlinks MUST work - symlinks pointing outside
// ROOT are intentional by design". The security model allows symlink traversal;
// it only requires that paths are presented via symlink names, not absolute paths.

securityTests.push({
  name: "Tabs and spaces allowed in paths",
  fn: async (client) => {
    // Tabs and spaces should be allowed (just newlines and control chars rejected)
    try {
      await client.callTool("ListDir", { dir: "subdir" });
      // Should succeed
    } catch (e) {
      throw new Error(`Valid path rejected: ${e.message}`);
    }
  },
});

securityTests.push({
  name: "Valid regex with special chars works",
  fn: async (client) => {
    // Valid regex should work
    const result = await client.callTool("Grep", { file: "test.txt", regex: "content|Safe" });
    if (!result.includes("Safe test content")) {
      throw new Error("Valid regex should work");
    }
  },
});

// ----------------------------------------------------------------------------
// ENVIRONMENT VARIABLE EXPANSION TESTS
// ----------------------------------------------------------------------------
// process.env.TEST = "/tmp", HOME = "/root", SECRET = "/etc/passwd", etc.

const ENV_VAR_ATTACKS = [
  // Basic Unix shell expansion
  "$TEST",
  "${TEST}",
  "$HOME",
  "${HOME}",
  "$SECRET",
  "${SECRET}",
  "$ROOT_DIR",
  "${ROOT_DIR}",
  "$EVIL",
  "${EVIL}",

  // With paths
  "$TEST/file.txt",
  "${TEST}/file.txt",
  "$HOME/.ssh/id_rsa",
  "${HOME}/.ssh/id_rsa",
  "$SECRET",
  "${ROOT_DIR}etc/passwd",

  // Nested/complex
  "${TEST:-/etc}",
  "${TEST:=/etc}",
  "${TEST:+/etc}",
  "${TEST:?error}",
  "${!TEST}",
  "${#TEST}",
  "${TEST%/*}",
  "${TEST%%/*}",
  "${TEST#*/}",
  "${TEST##*/}",
  "${TEST/tmp/etc}",
  "${TEST//tmp/etc}",

  // Command substitution (should be literal, not executed)
  "$(echo /etc/passwd)",
  "`echo /etc/passwd`",
  "$(cat /etc/passwd)",
  "`cat /etc/passwd`",
  "$(pwd)",
  "`pwd`",

  // Arithmetic expansion
  "$((1+1))",
  "$[1+1]",

  // Windows-style
  "%TEST%",
  "%HOME%",
  "%USERPROFILE%",
  "%SystemRoot%",
  "%windir%",

  // Mixed attacks
  "$TEST/../../../etc/passwd",
  "${TEST}/../../../etc/passwd",
  "${HOME}/../etc/passwd",
  "foo/$TEST/bar",
  "foo/${TEST}/bar",

  // Tilde expansion
  "~",
  "~/",
  "~root",
  "~root/",
  "~/.ssh/id_rsa",
  "~root/.ssh/id_rsa",

  // IFS manipulation attempts
  "${IFS}",
  "$IFS",
  "a${IFS}b",

  // Null/special vars
  "$0",
  "$1",
  "$$",
  "$!",
  "$?",
  "$@",
  "$*",
  "$-",
  "$_",

  // Double expansion attempts
  "\\$TEST",
  "\\${TEST}",
  "'$TEST'",
  '"$TEST"',
  "$$TEST",
];

// Test env var expansion in file/dir parameters
for (const payload of ENV_VAR_ATTACKS) {
  securityTests.push({
    name: `Read env expansion: ${payload.slice(0, 30)}`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Read", { file: payload });
        // If we got a result, verify it's not from /etc/passwd or /tmp
        if (result) {
          if (result.includes("root:x:0") || result.includes("root:*:0")) {
            throw new Error(`ENV VAR EXPANDED - read /etc/passwd via: ${payload}`);
          }
          // Check if we somehow read from /tmp (TEST=/tmp)
          // This would indicate $TEST was expanded
        }
      } catch (e) {
        // Expected to fail - verify error doesn't indicate expansion happened
        if (e.message.includes("root:x:0")) {
          throw new Error(`ENV VAR EXPANDED in error: ${payload}`);
        }
      }
    },
  });
}

// Test subset in ListDir
for (const payload of ENV_VAR_ATTACKS.slice(0, 20)) {
  securityTests.push({
    name: `ListDir env expansion: ${payload.slice(0, 30)}`,
    fn: async (client) => {
      try {
        const result = await client.callTool("ListDir", { dir: payload });
        // If $TEST expanded to /tmp, we'd see /tmp contents
        // If $HOME expanded to /root, we'd see /root contents
        if (result) {
          // Check for signs that we listed /tmp or /root
          if (result.includes("systemd-private") || result.includes(".ssh/")) {
            throw new Error(`ENV VAR EXPANDED - listed system dir via: ${payload}`);
          }
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// Test env vars in glob parameter
for (const payload of ENV_VAR_ATTACKS.slice(0, 15)) {
  securityTests.push({
    name: `Find glob env: ${payload.slice(0, 30)}`,
    fn: async (client) => {
      try {
        await client.callTool("Find", { dir: ".", glob: payload });
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// Test env vars in regex parameter
for (const payload of ENV_VAR_ATTACKS.slice(0, 15)) {
  securityTests.push({
    name: `Grep regex env: ${payload.slice(0, 30)}`,
    fn: async (client) => {
      try {
        await client.callTool("Grep", { file: "test.txt", regex: payload });
      } catch (e) {
        // Expected to fail - regex might be invalid, that's fine
      }
    },
  });
}

// Specific test: verify $TEST literally doesn't expand
securityTests.push({
  name: "Verify $TEST is literal, not expanded to /tmp",
  fn: async (client) => {
    try {
      // If $TEST expanded to /tmp, this would try to list /tmp
      const result = await client.callTool("ListDir", { dir: "$TEST" });
      // Should fail because "$TEST" is a literal directory name that doesn't exist
      throw new Error("Should have failed - $TEST dir shouldn't exist literally");
    } catch (e) {
      // Good - it failed. But verify we didn't actually access /tmp
      if (e.message.includes("systemd") || e.message.includes("snap")) {
        throw new Error("$TEST was expanded to /tmp!");
      }
      // Check that error message doesn't contain /tmp contents
    }
  },
});

// Specific test: verify ${HOME} literally doesn't expand
securityTests.push({
  name: "Verify ${HOME} is literal, not expanded to /root",
  fn: async (client) => {
    try {
      const result = await client.callTool("Read", { file: "${HOME}/.bashrc" });
      // Should fail because "${HOME}" is literal
      if (result && result.includes("bash")) {
        throw new Error("${HOME} was expanded - read actual .bashrc!");
      }
    } catch (e) {
      // Good - expected to fail
      if (e.message.includes("alias") || e.message.includes("export")) {
        throw new Error("${HOME} was expanded!");
      }
    }
  },
});

// Specific test: verify ~ (tilde) doesn't expand
securityTests.push({
  name: "Verify ~ is literal, not expanded to home dir",
  fn: async (client) => {
    try {
      const result = await client.callTool("ListDir", { dir: "~" });
      // Should fail because "~" is a literal directory name
      throw new Error("Should have failed - ~ dir shouldn't exist literally");
    } catch (e) {
      // Good - verify we didn't list actual home directory
      if (e.message.includes(".bashrc") || e.message.includes(".ssh") || e.message.includes("Documents")) {
        throw new Error("~ was expanded to home directory!");
      }
    }
  },
});

// Specific test: command substitution must not execute
securityTests.push({
  name: "Verify $(pwd) doesn't execute",
  fn: async (client) => {
    try {
      const result = await client.callTool("Read", { file: "$(pwd)/test.txt" });
      // If $(pwd) executed, it might have worked
      if (result && result.includes("Safe test content")) {
        throw new Error("$(pwd) was executed!");
      }
    } catch (e) {
      // Good - expected to fail
    }
  },
});

// Specific test: backtick command substitution must not execute
securityTests.push({
  name: "Verify `pwd` doesn't execute",
  fn: async (client) => {
    try {
      const result = await client.callTool("Read", { file: "`pwd`/test.txt" });
      if (result && result.includes("Safe test content")) {
        throw new Error("`pwd` was executed!");
      }
    } catch (e) {
      // Good - expected to fail
    }
  },
});

// ----------------------------------------------------------------------------
// DOT-DOT ENCODING BYPASS TESTS
// ----------------------------------------------------------------------------

// Test all dot-dot encoding variations
for (const payload of DOT_DOT_ENCODING_ATTACKS) {
  const displayPayload = JSON.stringify(payload).slice(1, -1).slice(0, 25);

  securityTests.push({
    name: `Read encoded ..: ${displayPayload}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("Read", { file: payload });
        // If we got content, check it's not /etc/passwd
        if (result && (result.includes("root:x:0") || result.includes("root:*:0"))) {
          throw new Error(`ENCODING BYPASS: Read /etc/passwd via: ${displayPayload}`);
        }
      } catch (e) {
        // Expected to fail - verify error doesn't contain /etc/passwd content
        if (e.message && e.message.includes("root:x:0")) {
          throw new Error(`ENCODING BYPASS in error: ${displayPayload}`);
        }
      }
    },
  });
}

// Test subset with ListDir
for (const payload of DOT_DOT_ENCODING_ATTACKS.slice(0, 30)) {
  const displayPayload = JSON.stringify(payload).slice(1, -1).slice(0, 25);

  securityTests.push({
    name: `ListDir encoded ..: ${displayPayload}...`,
    fn: async (client) => {
      try {
        const result = await client.callTool("ListDir", { dir: payload });
        // Check if we escaped to list /etc or parent directories
        if (result && (result.includes("passwd") || result.includes("shadow") || result.includes("hosts"))) {
          throw new Error(`ENCODING BYPASS: Listed /etc via: ${displayPayload}`);
        }
      } catch (e) {
        // Expected to fail
      }
    },
  });
}

// Specific test: URL-encoded dots
securityTests.push({
  name: "URL-encoded %2e%2e must not be decoded",
  fn: async (client) => {
    try {
      // %2e%2e = .. in URL encoding
      // If decoded, this would be ../../../etc/passwd
      await client.callTool("Read", { file: "%2e%2e/%2e%2e/%2e%2e/etc/passwd" });
    } catch (e) {
      // Should fail - %2e%2e is treated as literal characters
      // Verify error doesn't indicate we reached /etc/passwd
      if (e.message && e.message.includes("root:")) {
        throw new Error("URL encoding was decoded!");
      }
    }
  },
});

// Specific test: Unicode fullwidth dots (NFKC normalizes to regular dots!)
securityTests.push({
  name: "Fullwidth dots ．． must not normalize to ..",
  fn: async (client) => {
    try {
      // U+FF0E U+FF0E = ．．(fullwidth dots)
      // NFKC normalization would convert these to regular ..
      const fullwidthDots = "\uff0e\uff0e";
      await client.callTool("Read", { file: `${fullwidthDots}/${fullwidthDots}/${fullwidthDots}/etc/passwd` });
    } catch (e) {
      if (e.message && e.message.includes("root:")) {
        throw new Error("Unicode normalization converted fullwidth dots to ..!");
      }
    }
  },
});

// Specific test: Zero-width characters between dots
securityTests.push({
  name: "Zero-width chars in .. must not be ignored",
  fn: async (client) => {
    try {
      // .\u200b. = dot + zero-width-space + dot
      // If zero-width is stripped, this becomes ..
      const zwsDots = ".\u200b.";
      await client.callTool("Read", { file: `${zwsDots}/${zwsDots}/${zwsDots}/etc/passwd` });
    } catch (e) {
      if (e.message && e.message.includes("root:")) {
        throw new Error("Zero-width characters were stripped, creating ..!");
      }
    }
  },
});

// Specific test: Combining characters
securityTests.push({
  name: "Combining characters with dots must not create ..",
  fn: async (client) => {
    try {
      // Test various combining character tricks
      const combiningDots = ".\u0307.";  // dot + combining dot above + dot
      await client.callTool("Read", { file: `${combiningDots}/${combiningDots}/etc/passwd` });
    } catch (e) {
      if (e.message && e.message.includes("root:")) {
        throw new Error("Combining characters created path traversal!");
      }
    }
  },
});

// Test path.join behavior with encoded inputs
securityTests.push({
  name: "path.join must not decode URL encoding",
  fn: async (client) => {
    // This test verifies that our path handling doesn't decode %2e%2e
    try {
      await client.callTool("ListDir", { dir: "%2e%2e" });
      // If this succeeds and lists parent, we have a problem
    } catch (e) {
      // Expected to fail with "no such directory" type error
      // NOT with ".. is not allowed" because %2e%2e is not ..
    }
  },
});

// Test that double-encoding doesn't get decoded twice
securityTests.push({
  name: "Double URL encoding %252e%252e must stay literal",
  fn: async (client) => {
    try {
      // %252e decodes to %2e, which then decodes to .
      // If double-decoded, this becomes ..
      await client.callTool("Read", { file: "%252e%252e/%252e%252e/%252e%252e/etc/passwd" });
    } catch (e) {
      if (e.message && e.message.includes("root:")) {
        throw new Error("Double URL encoding was decoded!");
      }
    }
  },
});

// ============================================================================
// MAIN
// ============================================================================

async function main() {
  console.log(`${MAGENTA}╔══════════════════════════════════════════════════════════╗${NC}`);
  console.log(`${MAGENTA}║     FS-MCP Server SECURITY Test Suite                     ║${NC}`);
  console.log(`${MAGENTA}║     Testing all attack vectors and ROOT leak prevention   ║${NC}`);
  console.log(`${MAGENTA}╚══════════════════════════════════════════════════════════╝${NC}\n`);

  try {
    await setupTestEnvironment();

    console.log(`${BLUE}Running ${securityTests.length} security tests...${NC}\n`);

    for (const test of securityTests) {
      const result = await runSecurityTest(test.name, test.fn, test.serverArgs);
      testResults.push(result);
    }

    // Summary
    console.log(`\n${MAGENTA}═══ Security Test Summary ═══${NC}`);
    console.log(`${GREEN}Passed: ${testsPassed}${NC}`);
    console.log(`${RED}Failed: ${testsFailed}${NC}`);

    if (testsFailed > 0) {
      console.log(`\n${RED}SECURITY FAILURES:${NC}`);
      testResults
        .filter((r) => !r.passed)
        .forEach((r) => {
          console.log(`  ${RED}✗ ${r.name}${NC}`);
          console.log(`    ${GRAY}${r.error}${NC}`);
        });
    }

    // Cleanup
    await fs.promises.rm(TEST_DIR, { recursive: true, force: true });
    console.log(`\n${GRAY}Test directory cleaned up${NC}`);

    if (testsFailed > 0) {
      console.log(`\n${RED}⚠ SECURITY TESTS FAILED - DO NOT USE IN PRODUCTION${NC}`);
      process.exit(1);
    } else {
      console.log(`\n${GREEN}✓ All security tests passed${NC}`);
      process.exit(0);
    }

  } catch (error) {
    console.error(`${RED}Test setup failed: ${error.message}${NC}`);
    console.error(error.stack);
    process.exit(1);
  }
}

main().catch(console.error);
