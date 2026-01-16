#!/usr/bin/env node
/**
 * Minimal MCP stdio server using rg/tree/ls commands.
 *
 * Usage: node fs-mcp-server.js [--no-rgrep-root] [--no-tree-root] <ROOT_DIR>
 * - All tool paths are relative to <ROOT_DIR>
 * - '..' segments are forbidden in any input path
 * - Absolute paths are not allowed
 *
 * Tools
 * - ListDir(dir): list children of dir using `ls -la`
 * - Tree(dir): recursively list using `tree` command
 * - Find(dir, glob): find entries using `rg --files --glob`
 * - Read(file, start, lines, headOrTail): read by lines with binary UTF-8 decode
 * - Grep(file, regex, before, after, caseSensitive): grep using `rg --json`
 * - RGrep(dir, regex, caseSensitive): recursive grep using `rg --json`
 */

import fs from "node:fs";
import path from "node:path";
import { spawn } from "node:child_process";

const fsp = fs.promises;
const STDIN = process.stdin;
const STDOUT = process.stdout;

const USAGE =
  "Usage: node fs-mcp-server.js [--no-rgrep-root] [--no-tree-root] <ROOT_DIR>";

function parseArgs(argv) {
  let allowRGrepRoot = true;
  let allowTreeRoot = true;
  let rootArg;
  for (const arg of argv) {
    if (arg === "--no-rgrep-root") {
      allowRGrepRoot = false;
      continue;
    }
    if (arg === "--no-tree-root") {
      allowTreeRoot = false;
      continue;
    }
    if (arg.startsWith("-")) {
      console.error(`Unknown flag: ${arg}`);
      console.error(USAGE);
      process.exit(1);
    }
    if (rootArg === undefined) {
      rootArg = arg;
      continue;
    }
    console.error(`Unexpected extra argument: ${arg}`);
    console.error(USAGE);
    process.exit(1);
  }
  return { allowRGrepRoot, allowTreeRoot, rootArg };
}

const { allowRGrepRoot, allowTreeRoot, rootArg } = parseArgs(
  process.argv.slice(2),
);

if (!rootArg || typeof rootArg !== "string") {
  console.error(USAGE);
  process.exit(1);
}

const ROOT = path.resolve(rootArg);

try {
  const stat = fs.statSync(ROOT);
  if (!stat.isDirectory()) {
    console.error(`Root path is not a directory: ${ROOT}`);
    process.exit(1);
  }
} catch (e) {
  console.error(`Root path not found: ${ROOT}`);
  process.exit(1);
}

const ALLOW_RGREP_ROOT = allowRGrepRoot;
const ALLOW_TREE_ROOT = allowTreeRoot;

function assertNoDotDot(raw) {
  if (typeof raw !== "string") throw new Error("path must be a string");
  if (raw === "") return;
  if (
    raw.startsWith("/") ||
    raw.startsWith("\\") ||
    /^[A-Za-z]:[\\/]/.test(raw)
  ) {
    throw new Error("absolute paths are not allowed");
  }
  const parts = raw.split(/[\\/]+/);
  for (const p of parts) {
    if (p === "..") throw new Error('".." is not allowed');
  }
}

function resolveRel(raw) {
  assertNoDotDot(raw);
  return path.join(ROOT, raw === "." ? "" : raw);
}

function assertWithinRoot(absPath) {
  const rel = path.relative(ROOT, absPath);
  if (rel.startsWith("..")) {
    throw new Error("Path escapes root directory");
  }
}

function validateDir(dir) {
  if (typeof dir !== "string") throw new Error("dir must be a string");
  if (dir.length === 0) throw new Error("dir must be non-empty");
  if (dir.startsWith("/")) throw new Error("absolute paths not allowed");
  assertNoDotDot(dir);
}

function validateFile(file) {
  if (typeof file !== "string") throw new Error("file must be a string");
  if (file.length === 0) throw new Error("file must be non-empty");
  if (file.startsWith("/")) throw new Error("absolute paths not allowed");
  assertNoDotDot(file);
}

function validateRegex(pattern) {
  if (typeof pattern !== "string") throw new Error("regex must be a string");
  if (pattern.length > 10000) throw new Error("Regex pattern too long");
}

function validateGlob(glob) {
  if (typeof glob !== "string") throw new Error("glob must be a string");
}

async function execCommand(cmd, args, options = {}) {
  const { allowedExitCodes = [0] } = options;
  return new Promise((resolve, reject) => {
    const child = spawn(cmd, args, {
      stdio: ["pipe", "pipe", "pipe"],
      shell: false,
      cwd: ROOT,
    });

    let stdout = "";
    let stderr = "";

    child.stdout.on("data", (data) => {
      stdout += data.toString();
    });
    child.stderr.on("data", (data) => {
      stderr += data.toString();
    });

    child.on("error", (e) => {
      if (e.code === "ENOENT") {
        reject(new Error(`Command not found: ${cmd}`));
      } else {
        reject(e);
      }
    });

    child.on("close", (code) => {
      if (!allowedExitCodes.includes(code)) {
        const error = new Error(
          `Command failed: ${cmd} ${args.join(" ")}: ${stderr.trim()}`,
        );
        error.code = code;
        reject(error);
      } else {
        resolve(stdout);
      }
    });
  });
}

function decodeUtf8WithHexEscapes(buf) {
  const bytes = buf;
  let out = "";
  for (let i = 0; i < bytes.length; i++) {
    const b0 = bytes[i];
    if (b0 <= 0x7f) {
      out += String.fromCharCode(b0);
      continue;
    }
    let needed = 0;
    let codePoint = 0;
    if (b0 >= 0xc2 && b0 <= 0xdf) {
      needed = 1;
      codePoint = b0 & 0x1f;
    } else if (b0 >= 0xe0 && b0 <= 0xef) {
      needed = 2;
      codePoint = b0 & 0x0f;
    } else if (b0 >= 0xf0 && b0 <= 0xf4) {
      needed = 3;
      codePoint = b0 & 0x07;
    } else {
      out += `\\u${b0.toString(16).padStart(4, "0")}`;
      continue;
    }
    if (i + needed >= bytes.length) {
      out += `\\u${b0.toString(16).padStart(4, "0")}`;
      continue;
    }
    let valid = true;
    for (let j = 1; j <= needed; j++) {
      const bj = bytes[i + j];
      if ((bj & 0xc0) !== 0x80) {
        valid = false;
        break;
      }
      codePoint = (codePoint << 6) | (bj & 0x3f);
    }
    if (!valid) {
      out += `\\u${b0.toString(16).padStart(4, "0")}`;
      continue;
    }
    if (codePoint >= 0xd800 && codePoint <= 0xdfff) {
      out += `\\u${b0.toString(16).padStart(4, "0")}`;
      continue;
    }
    if (codePoint <= 0xffff) {
      out += String.fromCharCode(codePoint);
    } else {
      codePoint -= 0x10000;
      out += String.fromCharCode(0xd800 + (codePoint >> 10));
      out += String.fromCharCode(0xdc00 + (codePoint & 0x3ff));
    }
    i += needed;
  }
  return out;
}

function formatLineNumber(n) {
  return String(n).padStart(4, " ") + " ";
}
function splitLinesPreserve(str) {
  const raw = str.split("\n");
  return raw.map((s) => (s.endsWith("\r") ? s.slice(0, -1) : s));
}

async function toolListDir(args) {
  const { dir, showSize, showLastModified } = args;
  validateDir(dir);
  const abs = resolveRel(dir);
  assertWithinRoot(abs);

  const cmd = "ls";
  const cmdArgs = ["-la", "--time-style=iso", abs];
  const output = await execCommand(cmd, cmdArgs);

  const lines = output.split("\n");
  const entries = [];

  for (const line of lines) {
    const match = line.match(
      /^([dlcbps-])([rwx-]{9})\s+(\d+)\s+(\S+)\s+(\S+)\s+(\d+)\s+(\d{2}-\d{2})\s+(\d{2}:\d{2})\s+(.+)$/,
    );
    if (match) {
      const typeChar = match[1];
      const size = parseInt(match[6], 10);
      const date = match[7];
      const time = match[8];
      let name = match[9];
      const mtime = `${date} ${time}`;

      if (name === "." || name === "..") continue;

      let type = "file";
      if (typeChar === "d") {
        type = "dir";
      } else if (typeChar === "l") {
        // For symlinks: strip " -> target" and check if target is a directory
        const arrowIdx = name.indexOf(" -> ");
        if (arrowIdx !== -1) {
          name = name.substring(0, arrowIdx);
        }
        // Check if symlink points to a directory
        const symlinkPath = path.join(abs, name);
        try {
          const stat = fs.statSync(symlinkPath); // follows symlinks
          type = stat.isDirectory() ? "dir" : "file";
        } catch {
          type = "file"; // broken symlink, treat as file
        }
      }

      let entry = type === "dir" ? name + "/" : name;

      const meta = [];
      if (showSize && type === "file") meta.push(`${size}B`);
      if (showLastModified) meta.push(`mod:${mtime}`);
      if (meta.length > 0) entry += ` (${meta.join(", ")})`;

      entries.push(entry);
    }
  }

  const fileCount = entries.filter((e) => !e.endsWith("/")).length;
  const dirCount = entries.filter((e) => e.endsWith("/")).length;
  const summary = `\n${fileCount} file${fileCount !== 1 ? "s" : ""} and ${dirCount} director${dirCount !== 1 ? "ies" : "y"} in ${dir}`;
  return entries.join("\n") + summary;
}

async function toolTree(args) {
  const { dir, showSize } = args;
  validateDir(dir);
  if (dir === "." && !ALLOW_TREE_ROOT) {
    throw new Error(
      "recursive tree on the entire root is disabled by --no-tree-root. Use it on specific subdirectories or start the server without --no-tree-root.",
    );
  }
  const abs = resolveRel(dir);
  assertWithinRoot(abs);

  const cmd = "tree";
  // Use -l to follow symlinks into directories
  const cmdArgs = ["-l", abs];
  let output = await execCommand(cmd, cmdArgs);

  // Strip symlink targets " -> /path/to/target" to hide external paths
  // Matches: "name -> /absolute/path" or "name -> relative/path"
  output = output.replace(/ -> [^\n]+/g, "");

  const fileMatch = output.match(/(\d+)\s+files?/);
  const dirMatch = output.match(/(\d+)\s+director(?:y|ies)?/);
  const files = fileMatch ? parseInt(fileMatch[1], 10) : 0;
  const dirs = dirMatch ? parseInt(dirMatch[1], 10) : 0;

  return (
    output +
    `\n${files} file${files !== 1 ? "s" : ""} and ${dirs} director${dirs !== 1 ? "ies" : "y"} under ${dir}`
  );
}

async function toolFind(args) {
  const { dir, glob } = args;
  validateDir(dir);
  validateGlob(glob);
  const abs = resolveRel(dir);
  assertWithinRoot(abs);

  const cmd = "rg";
  // Use relative path (dir) instead of absolute (abs) so rg outputs relative paths
  // Security validation already done via assertWithinRoot(abs)
  const cmdArgs = ["--files", "--glob", glob, dir];
  const output = await execCommand(cmd, cmdArgs);

  const paths = output.split("\n").filter((p) => p.trim());
  const summary = `${paths.length} file${paths.length !== 1 ? "s" : ""} matched under ${dir}`;
  return paths.length > 0 ? paths.join("\n") + "\n\n" + summary : summary;
}

async function toolRead(args) {
  const { file, start, lines, headOrTail } = args;
  validateFile(file);
  if (!Number.isInteger(start) || start < 0)
    throw new Error("start must be a non-negative integer");
  if (!Number.isInteger(lines) || lines < 1)
    throw new Error("lines must be an integer >= 1");
  const mode = headOrTail === undefined ? "head" : headOrTail;
  if (mode !== "head" && mode !== "tail")
    throw new Error("headOrTail must be 'head' or 'tail'");

  const abs = resolveRel(file);
  assertWithinRoot(abs);

  let stat;
  try {
    stat = await fsp.stat(abs);
  } catch (e) {
    throw new Error(`${file}: no such file`);
  }
  if (!stat.isFile()) throw new Error(`${file}: not a regular file`);

  const buf = await fsp.readFile(abs);
  const text = decodeUtf8WithHexEscapes(buf);
  const arr = splitLinesPreserve(text);
  const total = arr.length;
  let begin = 0;
  let end = 0;
  if (mode === "head") {
    begin = Math.min(start, total);
    end = Math.min(start + lines, total);
  } else {
    const skipFromEnd = Math.min(start, total);
    end = total - skipFromEnd;
    begin = Math.max(0, end - lines);
  }
  const numbered = [];
  for (let i = begin; i < end; i++)
    numbered.push(formatLineNumber(i + 1) + arr[i]);

  const header = `${file.replace(/\\/g, "/")} [${mode} ${begin + 1}-${end}/${total}]`;
  return header + "\n" + numbered.join("\n");
}

async function toolGrep(args) {
  const { file, regex, before, after, caseSensitive } = args;
  validateFile(file);
  validateRegex(regex);
  const abs = resolveRel(file);
  assertWithinRoot(abs);

  const beforeVal = before === undefined ? 10 : before;
  const afterVal = after === undefined ? 10 : after;
  const cs = caseSensitive === undefined ? false : caseSensitive;

  const cmd = "rg";
  const cmdArgs = [
    "-n",
    "-B",
    String(beforeVal),
    "-A",
    String(afterVal),
    ...(cs ? [] : ["-i"]),
    "--",
    regex,
    abs,
  ];

  const output = await execCommand(cmd, cmdArgs, { allowedExitCodes: [0, 1] });
  return parseRgTextToGrepFormat(output);
}

function parseRgTextToGrepFormat(output) {
  const lines = output.split("\n").filter((l) => l.trim());
  if (lines.length === 0) {
    return "0 matches found";
  }

  const outputLines = [];
  for (const line of lines) {
    const match = line.match(/^(\d+):(.*)$/);
    if (match) {
      const lineNum = parseInt(match[1], 10);
      const content = match[2];
      outputLines.push(`${String(lineNum).padStart(4, " ")}→${content}`);
    } else {
      outputLines.push(line);
    }
  }

  return outputLines.join("\n");
}

function parseRgJsonToRGrepFormat(output) {
  const lines = output.split("\n").filter((l) => l.trim());
  const events = lines
    .map((l) => {
      try {
        return JSON.parse(l);
      } catch {
        return null;
      }
    })
    .filter((e) => e && e.type === "match");

  const files = new Set(events.map((e) => e.data.path.text));
  const fileList = Array.from(files);

  if (fileList.length === 0) {
    return "0 matches found";
  }

  const summary = `${fileList.length} match${fileList.length !== 1 ? "es" : ""} found in ${fileList.length} file${fileList.length !== 1 ? "s" : ""}`;
  const header = `Files matched under .:`;

  return `${summary}\n\n${header}\n\n${fileList.join("\n")}`;
}

async function toolRGrep(args) {
  const { dir, regex, caseSensitive } = args;
  validateDir(dir);
  validateRegex(regex);
  if (dir === "." && !ALLOW_RGREP_ROOT) {
    throw new Error(
      "recursive grep on the entire tree from the root is disabled by --no-rgrep-root. Use it on specific subdirectories or start the server without --no-rgrep-root.",
    );
  }

  const abs = resolveRel(dir);
  assertWithinRoot(abs);
  const cs = caseSensitive === undefined ? false : caseSensitive;

  const cmd = "rg";
  // Use relative path (dir) instead of absolute (abs) so rg outputs relative paths
  // Security validation already done via assertWithinRoot(abs)
  const cmdArgs = ["--json", ...(cs ? [] : ["-i"]), "--", regex, dir];

  const output = await execCommand(cmd, cmdArgs, { allowedExitCodes: [0, 1] });
  return parseRgJsonToRGrepFormat(output);
}

const tools = [
  {
    name: "ListDir",
    description:
      'List contents of a directory using `ls -la`. Paths are relative to ROOT; ".." and absolute paths are forbidden.',
    inputSchema: {
      type: "object",
      properties: {
        dir: {
          type: "string",
          minLength: 1,
          description:
            'Directory path relative to ROOT (non-empty; use "." for root)',
        },
        showSize: {
          type: "boolean",
          default: false,
          description: "Include file sizes in bytes",
        },
        showLastModified: {
          type: "boolean",
          default: false,
          description: "Include last modified timestamp",
        },
      },
      required: ["dir"],
    },
    handler: toolListDir,
  },
  {
    name: "Tree",
    description: ALLOW_TREE_ROOT
      ? 'Recursively list files and directories using `tree`. Paths are relative to ROOT; ".." and absolute paths are forbidden.'
      : 'Recursively list files and directories using `tree`. Root "." is not allowed due to its size; use specific subdirectories.',
    inputSchema: {
      type: "object",
      properties: {
        dir: {
          type: "string",
          minLength: 1,
          description: ALLOW_TREE_ROOT
            ? "Directory path relative to ROOT"
            : 'Directory path relative to ROOT (root "." not allowed)',
        },
        showSize: {
          type: "boolean",
          default: false,
          description: "Include file sizes in bytes",
        },
      },
      required: ["dir"],
    },
    handler: toolTree,
  },
  {
    name: "Find",
    description:
      'Find entries by glob pattern using `rg --files`. Returns files only (no directories or symlinks). Paths are relative to ROOT; ".." and absolute paths are forbidden.',
    inputSchema: {
      type: "object",
      properties: {
        dir: {
          type: "string",
          minLength: 1,
          description: "Directory path relative to ROOT",
        },
        glob: {
          type: "string",
          description: "Simple glob pattern (* and ? only)",
        },
      },
      required: ["dir", "glob"],
    },
    handler: toolFind,
  },
  {
    name: "Read",
    description:
      "Read file by lines with numbering. Lines are multi-byte aware; invalid UTF-8 bytes are hex-escaped as \\u00NN. Each line is prefixed with a 4-char right-aligned line number and a space.",
    inputSchema: {
      type: "object",
      properties: {
        file: { type: "string", description: "File path relative to ROOT" },
        start: {
          type: "integer",
          minimum: 0,
          description:
            "Starting line (0-based for head, offset from end for tail)",
        },
        lines: {
          type: "integer",
          minimum: 1,
          description: "Number of lines to read",
        },
        headOrTail: {
          type: "string",
          enum: ["head", "tail"],
          default: "head",
          description: "'head' reads from start, 'tail' reads from end",
        },
      },
      required: ["file", "start", "lines"],
    },
    handler: toolRead,
  },
  {
    name: "Grep",
    description:
      'Grep in a file using `rg`. Returns matches with line numbers. Paths are relative to ROOT; ".." and absolute paths are forbidden.',
    inputSchema: {
      type: "object",
      properties: {
        file: { type: "string", description: "File path relative to ROOT" },
        regex: { type: "string", description: "Regular expression pattern" },
        before: {
          type: "integer",
          minimum: 0,
          default: 10,
          description: "Lines of context before match",
        },
        after: {
          type: "integer",
          minimum: 0,
          default: 10,
          description: "Lines of context after match",
        },
        caseSensitive: {
          type: "boolean",
          default: false,
          description: "true for case-sensitive matching",
        },
      },
      required: ["file", "regex"],
    },
    handler: toolGrep,
  },
  {
    name: "RGrep",
    description: ALLOW_RGREP_ROOT
      ? 'Recursive grep using `rg --json`. Returns paths of files that match. Paths are relative to ROOT; ".." and absolute paths are forbidden.'
      : 'Recursive grep using `rg --json`. Root "." is not allowed due to its size; use specific subdirectories.',
    inputSchema: {
      type: "object",
      properties: {
        dir: {
          type: "string",
          minLength: 1,
          description: ALLOW_RGREP_ROOT
            ? "Directory path relative to ROOT"
            : 'Directory path relative to ROOT (root "." not allowed)',
        },
        regex: { type: "string", description: "Regular expression pattern" },
        caseSensitive: {
          type: "boolean",
          default: false,
          description: "true for case-sensitive matching",
        },
      },
      required: ["dir", "regex"],
    },
    handler: toolRGrep,
  },
];

function listToolsResponse() {
  return {
    tools: tools.map((t) => ({
      name: t.name,
      description: t.description,
      inputSchema: t.inputSchema,
    })),
  };
}

function send(message) {
  const json = JSON.stringify(message);
  STDOUT.write(json + "\n");
}

function respond(id, result) {
  send({ jsonrpc: "2.0", id, result });
}
function respondError(id, code, message, data) {
  const error =
    data === undefined ? { code, message } : { code, message, data };
  send({ jsonrpc: "2.0", id, error });
}

let buffer = Buffer.alloc(0);

STDIN.on("data", (chunk) => {
  buffer = Buffer.concat([buffer, chunk]);
  processBuffer();
});

STDIN.on("end", () => {
  processBuffer();
  process.exit(0);
});

function processBuffer() {
  while (buffer.length > 0) {
    const nl = buffer.indexOf(0x0a);
    if (nl === -1) break;

    const lineBuf = buffer.subarray(0, nl);
    buffer = buffer.subarray(nl + 1);

    let lineStr = lineBuf.toString("utf8");
    if (lineStr.endsWith("\r")) lineStr = lineStr.slice(0, -1);
    if (lineStr.length === 0) continue;

    try {
      const msg = JSON.parse(lineStr);
      handleMessage(msg);
    } catch (e) {
      console.error("MCP: Failed to parse:", e.message);
    }
  }
}

function handleMessage(msg) {
  const { id, method, params } = msg;
  if (method === "initialize") {
    const result = {
      protocolVersion: "2024-11-05",
      serverInfo: { name: "fs-mcp", version: "0.2.0" },
      capabilities: { tools: {} },
    };
    respond(id, result);
    return;
  }
  if (method === "tools/list") {
    respond(id, listToolsResponse());
    return;
  }
  if (method === "tools/call") {
    const { name, arguments: args } = params || {};
    const tool = tools.find((t) => t.name === name);
    if (!tool) {
      respondError(id, -32601, `Unknown tool: ${name}`);
      return;
    }
    (async () => {
      try {
        const result = await tool.handler(args || {});
        respond(id, {
          content: [
            {
              type: "text",
              text:
                typeof result === "string"
                  ? result
                  : JSON.stringify(result, null, 2),
            },
          ],
        });
      } catch (e) {
        const message =
          e && typeof e.message === "string" ? e.message : "Tool error";
        respondError(id, -32000, message);
      }
    })();
    return;
  }
  if (method === "notifications/initialized") {
    return;
  }
  if (method === "notifications/cancelled") {
    return;
  }
  if (method === "prompts/list") {
    respond(id, { prompts: [] });
    return;
  }
  if (method === "prompts/get") {
    respond(id, { prompt: null });
    return;
  }
  if (method === "resources/list") {
    respond(id, { resources: [] });
    return;
  }
  if (method === "resources/read") {
    respond(id, { contents: [] });
    return;
  }
  if (method === "logging/setLevel") {
    respond(id, {});
    return;
  }
  if (method === "ping") {
    respond(id, {});
    return;
  }

  console.error("MCP: Unknown method:", String(method));
  if (id !== undefined)
    respondError(id, -32601, `Unknown method: ${String(method)}`);
}

STDIN.resume();
