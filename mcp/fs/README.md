# fs-mcp-server

A high-performance MCP (Model Context Protocol) server providing comprehensive filesystem operations with strong security, efficient token usage, and robust error handling.

## Overview

The `fs-mcp-server` is a production-ready filesystem server that implements the MCP protocol, enabling AI assistants to safely interact with local filesystems. It features:

- **Token-efficient output** - Compact markdown formatting reduces token usage by 60-75%
- **Strong security** - Path traversal prevention and root directory enforcement for requested paths (symlink targets are not validated)
- **Comprehensive operations** - Read, search, list, tree visualization, and grep capabilities
- **Robust error handling** - Graceful handling of binary files and invalid UTF-8
- **Rich metadata** - File sizes, timestamps, and operation statistics (symlink targets shown when underlying tools provide them)
- **Exclusions** - Root-level `.mcpignore` for admin-controlled ignores

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd mcp/fs

# Install dependencies
npm install

# Run tests
node test-fs-mcp.js
```

## Usage

### Starting the Server

```bash
# Serve current directory
node fs-mcp-server.js

# Serve specific directory
node fs-mcp-server.js /path/to/directory

# Disable root RGrep (safer for large trees)
node fs-mcp-server.js --no-rgrep-root /path/to/directory

# Disable root Tree (safer for large trees)
node fs-mcp-server.js --no-tree-root /path/to/directory

# Enable RGrep binary decoding (slower; hex-escapes invalid bytes)
node fs-mcp-server.js --rgrep-decode-binary /path/to/directory

# Use environment variable
MCP_ROOT=/path/to/directory node fs-mcp-server.js
```

### Security Model

The server enforces strict security boundaries:

1. **Root Directory Confinement** - All operations are restricted to the specified root directory
2. **Path Traversal Prevention** - Attempts to access parent directories (../) are blocked
3. **Symlink Note** - Symlink targets are not validated; resolved targets may point outside root depending on OS/tool defaults
4. **Absolute Path Rejection** - Absolute paths are rejected

## Available Tools

### ListDir

List directory contents with optional metadata.

**Parameters:**
- `dir` (string, required) - Directory path relative to root (use `.` for root; no leading slash)
- `showSize` (boolean, optional, default: false) - Include file sizes
- `showLastModified` (boolean, optional, default: false) - Include modification timestamps
- `showCreated` (boolean, optional, default: false) - Include creation timestamps

**Output Format:**
```
file1.txt                   1.2K
file2.md                    456B
subdir/                        -
link -> target.txt         1.2K
broken_link -> target.txt      -

2 files, 2 directories in docs
```

### Tree

Display directory structure as an ASCII tree.

**Parameters:**
- `dir` (string, required) - Directory path relative to root (use `.` for root; no leading slash). Root can be disabled with `--no-tree-root`.
- `showSize` (boolean, optional, default: false) - Include file sizes in bytes (files only)

**Output Format:**
```
project/
├── src/
│   ├── index.js
│   └── utils.js
├── docs/
│   └── README.md
└── package.json

3 directories, 4 files
```

### Find

Find files and directories matching glob patterns.

**Parameters:**
- `dir` (string, required) - Starting directory (use `.` for root; no leading slash)
- `glob` (string, required) - Glob pattern (e.g., `*.js`, `**/*.md`)

**Features:**
- Supports standard glob patterns (`*`, `?`, `**`)
- Symlink handling follows `rg --files` defaults (targets are not shown)
- Includes statistics on files/directories examined

**Output Format:**
```
src/index.js
src/utils.js
docs/README.md
link_to_file -> target.txt

4 files matched under project, examined 10 files in 3 directories
```

### Read

Read file contents with line numbers.

**Parameters:**
- `file` (string, required) - File path relative to root
- `start` (number, required) - Starting line (0-based)
- `lines` (number, required, min: 1) - Number of lines to read
- `headOrTail` (string, optional, default: `head`) - Read from 'head' or 'tail'

**Features:**
- Line numbers for easy reference
- UTF-8 decoding with hex escape for invalid bytes
- Symlink following with target display
- Automatic file size warnings for large files

**Output Format:**
```
file.txt (100 lines, 2.5KB):
1│First line of content
2│Second line of content
3│Third line of content
```

### Grep

Search file contents with regex patterns.

**Parameters:**
- `file` (string, required) - File to search
- `regex` (string, required) - Regular expression pattern
- `caseSensitive` (boolean, optional, default: false) - Case sensitivity
- `before` (number, optional, default: 10) - Context lines before match
- `after` (number, optional, default: 10) - Context lines after match

**Features:**
- Multi-line regex support
- Context lines (before/after)
- Merged overlapping matches
- Arrow markers (→) for matched lines
- Match statistics

**Output Format:**
```
file.txt: 3 matches found. Total lines in file: 100
48 Line 48: Regular content
49 Line 49: Regular content
50→Line 50: This line MATCHES
51 Line 51: Regular content
52→Line 52: Another MATCH here
───
98→Line 98: Final MATCH
99 Line 99: Regular content
```

### RGrep

Recursively search directory contents.

**Parameters:**
- `dir` (string, required) - Directory to search (use `.` for root; no leading slash). Root can be disabled with `--no-rgrep-root`.
- `regex` (string, required) - Regular expression pattern
- `caseSensitive` (boolean, required) - Case sensitivity
- `maxFiles` (number, optional, default: 200) - Maximum matching files to return (search stops after this many matches)

**Features:**
- Recursive directory traversal
- Per-file match reporting
- Overall statistics
- If `maxFiles` is reached, output includes a warning that results are incomplete and suggests re-running with a higher limit

**Output Format:**
```
WARNING: RGrep stopped early because maxFiles=1 was reached. Results may be incomplete.
If you need all matches, re-run RGrep with a higher maxFiles value.
1 match found in 1 file across 3 directories under project (examined 15 files)

Files matched under project:

src/index.js
```

## Special Features

### Symlink Handling

Symlink handling is best-effort and delegated to underlying tools (`ls`, `tree`, `rg`):

- Symlink targets may be shown in listings when the tool provides them
- The server does **not** validate or restrict resolved symlink targets
- Read/Grep follow OS default symlink behavior
- Directory traversal behavior depends on `tree`/`rg` defaults
- No explicit symlink loop detection is performed

### Hidden Files

Hidden files (starting with `.`) are included by default in all operations:
- ListDir shows hidden files
- Find matches hidden files with wildcards
- Tree displays hidden entries
- RGrep searches hidden files

### Exclusions (.mcpignore)

Exclusions are admin-only and applied to **all tools**.

- Root-level `.mcpignore` file only (no nested ignore files)
- Uses full `.gitignore` pattern semantics
- If `.mcpignore` exists, **no internal defaults** are applied
- If `.mcpignore` is missing or unreadable, defaults apply: `.git/` and `node_modules/`
- Explicit access to excluded paths returns ENOENT; traversal skips excluded paths silently

### Binary File Handling

Binary files are handled gracefully:
- Read/Grep: invalid UTF-8 bytes are shown as hex escapes (e.g., `\xFE`)
- RGrep: uses plain UTF-8 decode by default; use `--rgrep-decode-binary` to enable hex-escape decoding
- Large files include size warnings
- Binary content remains searchable

### Error Messages

Clear, actionable error messages:
```
"not a regular file"
"no such file"
"recursive grep on the entire tree from the root is disabled by --no-rgrep-root"
```

## Performance Characteristics

- **Token Usage**: 60-75% reduction compared to JSON output
- **Memory Efficient**: Streaming reads for large files
- **Fast Searching**: Compiled regex patterns, efficient traversal
- **Scalable**: Handles large directory structures

## Testing

Comprehensive test suite with 35+ tests covering:
- Basic operations
- Symlink scenarios
- Hidden file handling
- Binary file processing
- Error conditions
- Path security
- Edge cases

Run tests:
```bash
node test-fs-mcp.js        # Basic output
node test-fs-mcp.js -v     # Verbose with examples
```

## Security Considerations

1. **Never run as root** - Use appropriate user permissions
2. **Validate root directory** - Ensure it contains intended files
3. **Monitor symlinks** - Review symlink warnings in output
4. **Limit scope** - Use most specific directory possible
5. **Review operations** - Check file counts in statistics

## Comparison with Standard Tools

| Feature | fs-mcp-server | Standard JSON MCP |
|---------|--------------|-------------------|
| Token usage | 25-40% | 100% baseline |
| Symlink info | Always shown | Often hidden |
| Statistics | All operations | Usually none |
| Binary files | Hex escapes | May fail |
| Hidden files | Included | Varies |
| Output format | Markdown | JSON |

## Contributing

When contributing, ensure:
1. All tests pass (`node test-fs-mcp.js`)
2. Security model is maintained
3. Output remains compact
4. Statistics are updated
5. Error messages are clear

## License

[Insert appropriate license]

## Support

For issues, questions, or contributions, please [open an issue](link-to-issues).
