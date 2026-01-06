#!/usr/bin/env node
/**
 * Test suite for fs-mcp-server.js (rg/tree/ls version)
 * Tests basic file operations, grep, tree, find, and edge cases.
 */

import fs from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import os from "node:os";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const TEST_DIR = path.join(os.tmpdir(), `fs-mcp-test-${Date.now()}`);
const SERVER_PATH = path.join(__dirname, "fs-mcp-server.js");

const RED = "\x1b[0;31m";
const GREEN = "\x1b[0;32m";
const YELLOW = "\x1b[1;33m";
const BLUE = "\x1b[0;34m";
const GRAY = "\x1b[0;90m";
const NC = "\x1b[0m";

let testsPassed = 0;
let testsFailed = 0;
const testResults = [];
let verbose = false;

if (process.argv.includes("--verbose") || process.argv.includes("-v")) {
  verbose = true;
  console.log(`${YELLOW}Running in verbose mode${NC}`);
}

async function setupTestEnvironment() {
  console.log(`${BLUE}Setting up test environment in: ${TEST_DIR}${NC}`);
  await fs.promises.mkdir(TEST_DIR, { recursive: true });
  await fs.promises.mkdir(path.join(TEST_DIR, "dir1"));
  await fs.promises.mkdir(path.join(TEST_DIR, "dir1", "subdir"));
  await fs.promises.mkdir(path.join(TEST_DIR, "empty_dir"));
  await fs.promises.mkdir(path.join(TEST_DIR, ".git"));
  await fs.promises.mkdir(path.join(TEST_DIR, ".git", "objects"));
  await fs.promises.writeFile(
    path.join(TEST_DIR, "file1.txt"),
    "Hello World\nLine 2\nLine 3",
  );
  await fs.promises.writeFile(
    path.join(TEST_DIR, "file2.md"),
    "# Header\n\nContent here",
  );
  await fs.promises.writeFile(
    path.join(TEST_DIR, "dir1", "nested.js"),
    "const x = 42;\nconsole.log(x);",
  );
  await fs.promises.writeFile(
    path.join(TEST_DIR, "dir1", "subdir", "deep.txt"),
    "Deep file content",
  );
  const binaryContent = Buffer.from([
    0xff, 0xfe, 0x00, 0x48, 0x65, 0x6c, 0x6c, 0x6f,
  ]);
  await fs.promises.writeFile(path.join(TEST_DIR, "binary.dat"), binaryContent);
  const lines = Array.from(
    { length: 100 },
    (_, i) => `Line ${i + 1}: ${i % 10 === 0 ? "MATCH" : "content"}`,
  );
  await fs.promises.writeFile(
    path.join(TEST_DIR, "hundred.txt"),
    lines.join("\n"),
  );
  await fs.promises.writeFile(
    path.join(TEST_DIR, "single.txt"),
    "One line only",
  );
  await fs.promises.writeFile(
    path.join(TEST_DIR, "special.txt"),
    "File with spaces in name",
  );
}

class MCPTestClient {
  constructor(serverArgs = []) {
    this.server = null;
    this.buffer = "";
    this.messageId = 1;
    this.pendingRequests = new Map();
    this.serverArgs = Array.isArray(serverArgs) ? serverArgs : [];
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
        if (verbose) console.error(`${GRAY}[stderr] ${data.toString()}${NC}`);
      });

      this.server.on("error", reject);

      this.sendRequest("initialize", {
        protocolVersion: "2024-11-05",
        clientInfo: { name: "test-client", version: "1.0.0" },
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
        if (msg.id && this.pendingRequests.has(msg.id)) {
          const { resolve, reject } = this.pendingRequests.get(msg.id);
          this.pendingRequests.delete(msg.id);
          if (msg.error) reject(new Error(msg.error.message));
          else resolve(msg.result);
        }
      } catch (e) {}
    }
  }

  sendRequest(method, params) {
    return new Promise((resolve, reject) => {
      const id = this.messageId++;
      this.pendingRequests.set(id, { resolve, reject });
      const message =
        JSON.stringify({ jsonrpc: "2.0", id, method, params }) + "\n";
      this.server.stdin.write(message);
    });
  }

  async callTool(name, args) {
    const result = await this.sendRequest("tools/call", {
      name,
      arguments: args,
    });
    return result?.content?.[0]?.text || result;
  }
}

async function runTest(name, testFn, serverArgs = []) {
  const testNum = testsPassed + testsFailed + 1;

  if (verbose) {
    console.log(`\n${BLUE}═══ Test ${testNum}: ${name} ═══${NC}`);
  } else {
    process.stdout.write(`${GRAY}[${testNum}] ${name}...${NC} `);
  }

  const client = new MCPTestClient(serverArgs);
  try {
    await client.start();
    await testFn(client);
    testsPassed++;
    if (verbose) console.log(`${GREEN}✓ PASSED${NC}`);
    else console.log(`${GREEN}✓${NC}`);
    return { name, passed: true };
  } catch (error) {
    testsFailed++;
    if (verbose) console.log(`${RED}✗ FAILED: ${error.message}${NC}`);
    else console.log(`${RED}✗${NC}`);
    return { name, passed: false, error: error.message };
  } finally {
    await client.stop();
  }
}

const tests = [
  {
    name: "ListDir basic",
    fn: async (client) => {
      const result = await client.callTool("ListDir", { dir: "." });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("dir1/")) throw new Error("Missing dir1");
      if (!result.includes("file1.txt")) throw new Error("Missing file1.txt");
      if (!result.includes(".git/"))
        throw new Error("Hidden .git should be shown");
    },
  },

  {
    name: "ListDir with size and mtime",
    fn: async (client) => {
      const result = await client.callTool("ListDir", {
        dir: ".",
        showSize: true,
        showLastModified: true,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir with metadata${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("file1.txt")) throw new Error("Missing file1.txt");
      if (!result.includes("B")) throw new Error("Missing size");
      if (!result.includes("mod:"))
        throw new Error("Missing modification time");
    },
  },

  {
    name: "Read regular file",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "file1.txt",
        start: 0,
        lines: 100,
        headOrTail: "head",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Hello World")) throw new Error("Content mismatch");
      if (!result.includes("Line 2")) throw new Error("Missing line 2");
    },
  },

  {
    name: "Read with tail mode",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "file1.txt",
        start: 0,
        lines: 2,
        headOrTail: "tail",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read tail${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Line 2"))
        throw new Error("Should show last 2 lines");
      if (!result.includes("Line 3")) throw new Error("Should show Line 3");
    },
  },

  {
    name: "Read binary file",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "binary.dat",
        start: 0,
        lines: 10,
        headOrTail: "head",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read binary${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("\\u00"))
        throw new Error("Should show hex escapes for invalid UTF-8");
    },
  },

  {
    name: "Read tail with start > 0",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "hundred.txt",
        start: 5,
        lines: 3,
        headOrTail: "tail",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read tail with start${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Line 93"))
        throw new Error("Should show last 3 lines from 93");
    },
  },

  {
    name: "Read exact line count",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "file1.txt",
        start: 0,
        lines: 3,
        headOrTail: "head",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read exact lines${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Hello World"))
        throw new Error("Should show Hello World");
      if (!result.includes("Line 2")) throw new Error("Should show Line 2");
      if (!result.includes("Line 3")) throw new Error("Should show Line 3");
    },
  },

  {
    name: "Read with start > 0",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "hundred.txt",
        start: 5,
        lines: 3,
        headOrTail: "head",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read with start${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Line 6"))
        throw new Error("Should show starting from line 6");
      if (!result.includes("Line 7")) throw new Error("Should show line 7");
      if (!result.includes("Line 8")) throw new Error("Should show line 8");
    },
  },

  {
    name: "Read with start beyond lines",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "hundred.txt",
        start: 50,
        lines: 10,
        headOrTail: "head",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read start beyond file${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Line 51"))
        throw new Error("Should show starting from line 51");
    },
  },

  {
    name: "Tree basic",
    fn: async (client) => {
      const result = await client.callTool("Tree", { dir: "dir1" });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("nested.js")) throw new Error("Missing nested.js");
      if (!result.includes("subdir")) throw new Error("Missing subdir");
      if (!result.includes("deep.txt")) throw new Error("Missing deep.txt");
    },
  },

  {
    name: "Tree root allowed by default",
    fn: async (client) => {
      const result = await client.callTool("Tree", { dir: "." });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree root${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("dir1")) throw new Error("Tree should show dir1");
    },
  },

  {
    name: "Tree root blocked with --no-tree-root",
    serverArgs: ["--no-tree-root"],
    fn: async (client) => {
      try {
        await client.callTool("Tree", { dir: "." });
        throw new Error("Should have failed on root Tree with --no-tree-root");
      } catch (e) {
        if (verbose) console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        if (!String(e.message).includes("--no-tree-root")) {
          throw new Error("Error should mention --no-tree-root");
        }
      }
    },
  },

  {
    name: "Tree on empty directory",
    fn: async (client) => {
      const result = await client.callTool("Tree", { dir: "empty_dir" });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree empty dir${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("0 files"))
        throw new Error("Should show 0 files in empty directory");
    },
  },

  {
    name: "Tree with showSize",
    fn: async (client) => {
      const result = await client.callTool("Tree", {
        dir: "dir1",
        showSize: true,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Tree with size${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("nested.js")) throw new Error("Missing nested.js");
      if (!result.includes("subdir")) throw new Error("Missing subdir");
      if (!result.includes("deep.txt")) throw new Error("Missing deep.txt");
    },
  },

  {
    name: "Find with glob",
    fn: async (client) => {
      const result = await client.callTool("Find", {
        dir: "dir1",
        glob: "*.js",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Find${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("nested.js")) throw new Error("Missing nested.js");
    },
  },

  {
    name: "Find returns files only",
    fn: async (client) => {
      const result = await client.callTool("Find", { dir: ".", glob: "*" });
      if (verbose) {
        console.log(`${GRAY}Tool: Find files only${NC}`);
        console.log("Output:", result);
      }
      const lines = result.split("\n").filter((l) => l.trim());
      for (const line of lines) {
        if (line.endsWith("/"))
          throw new Error("Find should not include directories");
      }
      if (lines.length === 0) throw new Error("Find should return files");
    },
  },

  {
    name: "Find with broad glob",
    fn: async (client) => {
      const result = await client.callTool("Find", { dir: ".", glob: "*" });
      if (verbose) {
        console.log(`${GRAY}Tool: Find broad glob${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("file1.txt"))
        throw new Error("Should match file1.txt");
      if (!result.includes("nested.js"))
        throw new Error("Should match nested.js");
    },
  },

  {
    name: "Find non-existent directory",
    fn: async (client) => {
      try {
        await client.callTool("Find", {
          dir: "nonexistent_dir",
          glob: "*.txt",
        });
        throw new Error("Should have failed on non-existent directory");
      } catch (e) {
        if (verbose) console.log(`${GRAY}Expected error: ${e.message}${NC}`);
      }
    },
  },

  {
    name: "Grep basic",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "file1.txt",
        regex: "Line",
        caseSensitive: true,
        before: 0,
        after: 0,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Line 2")) throw new Error("Missing line 2");
      if (!result.includes("Line 3")) throw new Error("Missing line 3");
    },
  },

  {
    name: "Grep case insensitive",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "file1.txt",
        regex: "hello",
        caseSensitive: false,
        before: 0,
        after: 0,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep case insensitive${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Hello World"))
        throw new Error("Should match case-insensitive");
    },
  },

  {
    name: "Grep only before context",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "hundred.txt",
        regex: "Line 50",
        caseSensitive: true,
        before: 2,
        after: 0,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep only before context${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("48"))
        throw new Error("Missing before context line 48");
      if (!result.includes("49"))
        throw new Error("Missing before context line 49");
      if (!result.includes("50")) throw new Error("Missing matched line 50");
      if (result.includes("51"))
        throw new Error("Should not include after context");
    },
  },

  {
    name: "Grep only after context",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "hundred.txt",
        regex: "Line 50",
        caseSensitive: true,
        before: 0,
        after: 2,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep only after context${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("50")) throw new Error("Missing matched line 50");
      if (!result.includes("51"))
        throw new Error("Missing after context line 51");
      if (!result.includes("52"))
        throw new Error("Missing after context line 52");
      if (result.includes("49"))
        throw new Error("Should not include before context");
    },
  },

  {
    name: "Grep multiple matches",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "hundred.txt",
        regex: "MATCH",
        caseSensitive: true,
        before: 0,
        after: 0,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep multiple matches${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("11")) throw new Error("Missing match at line 11");
      if (!result.includes("21")) throw new Error("Missing match at line 21");
      if (!result.includes("31")) throw new Error("Missing match at line 31");
    },
  },

  {
    name: "Grep overlapping context",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "hundred.txt",
        regex: "Line 1",
        caseSensitive: true,
        before: 5,
        after: 5,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep overlapping context${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("1")) throw new Error("Should include line 1");
      if (!result.includes("2")) throw new Error("Should include line 2");
      if (!result.includes("3")) throw new Error("Should include line 3");
    },
  },

  {
    name: "Grep regex special chars",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "file1.txt",
        regex: "World$",
        caseSensitive: true,
        before: 0,
        after: 0,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep regex special chars${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Hello World"))
        throw new Error("Should match regex with special char");
    },
  },

  {
    name: "Grep with context",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "hundred.txt",
        regex: "Line 50",
        caseSensitive: true,
        before: 2,
        after: 3,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep with context${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("48"))
        throw new Error("Missing before context line 48");
      if (!result.includes("49"))
        throw new Error("Missing before context line 49");
      if (!result.includes("50")) throw new Error("Missing matched line 50");
      if (!result.includes("51"))
        throw new Error("Missing after context line 51");
    },
  },

  {
    name: "Grep no match",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "file1.txt",
        regex: "nonexistent",
        caseSensitive: true,
        before: 0,
        after: 0,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep no match${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("0 matches found"))
        throw new Error("Should indicate no matches");
    },
  },

  {
    name: "RGrep basic",
    fn: async (client) => {
      const result = await client.callTool("RGrep", {
        dir: "dir1",
        regex: "console",
        caseSensitive: true,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("nested.js")) throw new Error("Missing file path");
    },
  },

  {
    name: "RGrep case insensitive",
    fn: async (client) => {
      const result = await client.callTool("RGrep", {
        dir: "dir1",
        regex: "CONSOLE",
        caseSensitive: false,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep case insensitive${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("nested.js"))
        throw new Error("Should match case-insensitive");
    },
  },

  {
    name: "RGrep case sensitive",
    fn: async (client) => {
      const result = await client.callTool("RGrep", {
        dir: "dir1",
        regex: "console",
        caseSensitive: true,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep case sensitive${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("nested.js"))
        throw new Error("Should match case-sensitive");
    },
  },

  {
    name: "RGrep root allowed by default",
    fn: async (client) => {
      const result = await client.callTool("RGrep", {
        dir: ".",
        regex: "content",
        caseSensitive: true,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep root${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("Files matched under .:"))
        throw new Error("RGrep root should include the files header");
    },
  },

  {
    name: "RGrep root blocked with --no-rgrep-root",
    serverArgs: ["--no-rgrep-root"],
    fn: async (client) => {
      try {
        await client.callTool("RGrep", {
          dir: ".",
          regex: "content",
          caseSensitive: true,
        });
        throw new Error(
          "Should have failed on root RGrep with --no-rgrep-root",
        );
      } catch (e) {
        if (verbose) console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        if (!String(e.message).includes("--no-rgrep-root")) {
          throw new Error("Error should mention --no-rgrep-root");
        }
      }
    },
  },

  {
    name: "RGrep no match",
    fn: async (client) => {
      const result = await client.callTool("RGrep", {
        dir: "dir1",
        regex: "nonexistent_pattern",
        caseSensitive: true,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: RGrep no match${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("0 matches found"))
        throw new Error("Should indicate no matches");
    },
  },

  {
    name: "Path traversal prevention",
    fn: async (client) => {
      try {
        await client.callTool("Read", {
          file: "../../../etc/passwd",
          start: 0,
          lines: 100,
          headOrTail: "head",
        });
        throw new Error("Should have prevented path traversal");
      } catch (e) {
        if (verbose) console.log(`${GRAY}Correctly blocked: ${e.message}${NC}`);
      }
    },
  },

  {
    name: "Absolute path prevention",
    fn: async (client) => {
      try {
        await client.callTool("Read", {
          file: "/etc/passwd",
          start: 0,
          lines: 100,
          headOrTail: "head",
        });
        throw new Error("Should have prevented absolute path");
      } catch (e) {
        if (verbose) console.log(`${GRAY}Correctly blocked: ${e.message}${NC}`);
      }
    },
  },

  {
    name: "Empty directory listing",
    fn: async (client) => {
      const result = await client.callTool("ListDir", { dir: "empty_dir" });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir empty${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("0 files"))
        throw new Error("Should indicate empty directory");
    },
  },

  {
    name: "Non-existent file error",
    fn: async (client) => {
      try {
        await client.callTool("Read", {
          file: "nonexistent.txt",
          start: 0,
          lines: 100,
          headOrTail: "head",
        });
        throw new Error("Should have failed");
      } catch (e) {
        if (verbose) console.log(`${GRAY}Expected error: ${e.message}${NC}`);
      }
    },
  },

  {
    name: "Invalid regex in Grep",
    fn: async (client) => {
      try {
        await client.callTool("Grep", {
          file: "file1.txt",
          regex: "[",
          caseSensitive: true,
          before: 0,
          after: 0,
        });
        throw new Error("Should have failed on invalid regex");
      } catch (e) {
        if (verbose) console.log(`${GRAY}Expected error: ${e.message}${NC}`);
      }
    },
  },

  {
    name: "Regex pattern too long",
    fn: async (client) => {
      const longPattern = "a".repeat(10001);
      try {
        await client.callTool("Grep", {
          file: "file1.txt",
          regex: longPattern,
          caseSensitive: true,
          before: 0,
          after: 0,
        });
        throw new Error("Should have failed on overly long regex");
      } catch (e) {
        if (verbose) console.log(`${GRAY}Expected error: ${e.message}${NC}`);
        if (!String(e.message).includes("too long")) {
          throw new Error("Error should mention regex length limit");
        }
      }
    },
  },

  {
    name: "Find with question mark glob",
    fn: async (client) => {
      const result = await client.callTool("Find", {
        dir: ".",
        glob: "single.t?t",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Find ? glob${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("single.txt"))
        throw new Error("Should match single.txt with ? glob");
    },
  },

  {
    name: "Find glob matching nothing",
    fn: async (client) => {
      try {
        await client.callTool("Find", {
          dir: ".",
          glob: "*.nonexistent",
        });
        throw new Error("Should fail when glob matches nothing");
      } catch (e) {
        if (verbose) console.log(`${GRAY}Expected error: ${e.message}${NC}`);
      }
    },
  },

  {
    name: "Read single line file",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "single.txt",
        start: 0,
        lines: 10,
        headOrTail: "head",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read single line${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("One line only"))
        throw new Error("Should show the single line");
    },
  },

  {
    name: "Read lines exceed file length",
    fn: async (client) => {
      const result = await client.callTool("Read", {
        file: "single.txt",
        start: 0,
        lines: 100,
        headOrTail: "head",
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Read lines exceed file${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("One line only"))
        throw new Error("Should show available lines only");
    },
  },

  {
    name: "Grep case sensitive explicit",
    fn: async (client) => {
      const result = await client.callTool("Grep", {
        file: "file1.txt",
        regex: "hello",
        caseSensitive: true,
        before: 0,
        after: 0,
      });
      if (verbose) {
        console.log(`${GRAY}Tool: Grep case sensitive explicit${NC}`);
        console.log("Output:", result);
      }
      if (result.includes("Hello World"))
        throw new Error("Should not match case-sensitive");
    },
  },

  {
    name: "ListDir files only directory",
    fn: async (client) => {
      const result = await client.callTool("ListDir", { dir: "empty_dir" });
      if (verbose) {
        console.log(`${GRAY}Tool: ListDir files only${NC}`);
        console.log("Output:", result);
      }
      if (!result.includes("0 files"))
        throw new Error("Should show 0 files in empty directory");
    },
  },
];

async function main() {
  console.log(`${BLUE}╔════════════════════════════════════════╗${NC}`);
  console.log(`${BLUE}║   FS-MCP Server Tests (rg/tree/ls)      ║${NC}`);
  console.log(`${BLUE}╚════════════════════════════════════════╝${NC}`);

  try {
    await setupTestEnvironment();

    for (const test of tests) {
      const result = await runTest(test.name, test.fn, test.serverArgs);
      testResults.push(result);
    }

    console.log(`\n${BLUE}═══ Test Summary ═══${NC}`);
    console.log(`${GREEN}Passed: ${testsPassed}${NC}`);
    console.log(`${RED}Failed: ${testsFailed}${NC}`);

    if (testsFailed > 0) {
      console.log(`\n${RED}Failed tests:${NC}`);
      testResults
        .filter((r) => !r.passed)
        .forEach((r) => {
          console.log(`  ${RED}✗ ${r.name}: ${r.error}${NC}`);
        });
    }

    await fs.promises.rm(TEST_DIR, { recursive: true, force: true });
    console.log(`${GRAY}Test directory cleaned up${NC}`);

    process.exit(testsFailed > 0 ? 1 : 0);
  } catch (error) {
    console.error(`${RED}Test setup failed: ${error.message}${NC}`);
    process.exit(1);
  }
}

main().catch(console.error);
