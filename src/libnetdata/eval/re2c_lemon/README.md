# re2c/Lemon Parser for Netdata Expression Evaluator

This directory contains a parser implementation for Netdata's expression evaluator using re2c for lexical analysis and Lemon for syntax parsing.

## Overview

The re2c/Lemon-based parser is designed to be more efficient and maintainable compared to the handwritten recursive descent parser in the parent directory. It achieves full compatibility with the original parser while providing better performance, especially for complex expressions with nested operations.

## Components

- **lexer.re** - The re2c-based lexical analyzer source file
- **lexer.c** - The generated lexer code (generated from lexer.re)
- **parser.y** - The Lemon-based grammar definition
- **parser.c** - The generated parser code (generated from parser.y)
- **parser.h** - The generated parser header
- **parser_internal.h** - Internal definitions shared between the lexer and parser
- **parser_wrapper.c** - Integration wrapper for the Netdata build system
- **Makefile** - Build instructions for regenerating the parser

## Technology Stack

### re2c

[re2c](https://re2c.org/) is a lexer generator that translates regular expressions into deterministic finite automata (DFA) and produces efficient C code. The lexer implemented in `lexer.re` tokenizes the input string according to the expression language grammar, handling:

- Special literals (nan, inf)
- Numbers
- Variable names
- Operators
- Function names
- Whitespace

### Lemon

[Lemon](https://www.sqlite.org/lemon.html) is a parser generator similar to YACC/Bison but with a different parsing technique and better thread safety. The grammar in `parser.y` defines the expression language syntax, operator precedence, and associativity rules.

## Integration

This parser implementation can be selected by defining `USE_RE2C_LEMON_PARSER` in `eval-internal.h`. When enabled, the function `parse_expression_with_re2c_lemon()` is used instead of the original recursive descent parser.

## Key Features

1. **Parser Generator Approach**: Using specialized tools (re2c and Lemon) for lexical analysis and parsing rather than handwritten code.

2. **Full Compatibility**: Maintains 100% compatibility with the original parser, ensuring all test cases pass with identical results.

3. **Proper Operator Precedence**: Handles operator precedence correctly, especially for complex cases like nested ternary operators.

4. **Flexible Variable Names**: Supports the same variable naming rules as the original parser, including braced variables with spaces.

5. **Special Literal Handling**: Properly processes special literals like NaN and Infinity in various capitalizations.

6. **Case-Insensitive Keywords**: Handles logical operators (AND, OR, NOT) case-insensitively, just like the original parser.

7. **Reduced Memory Leaks**: Carefully manages memory allocations to prevent leaks, especially in error conditions.

## Rebuilding the Parser

If you need to modify the lexer or parser definitions, you can rebuild the generated files using:

```bash
make -C src/libnetdata/eval/re2c_lemon
```

This requires re2c and lemon to be installed on your system.

## Technical Details

### Parser Notes

- The parser builds an abstract syntax tree (AST) using the `EVAL_NODE` structure.
- Operator precedence is carefully defined to match C-like languages.
- The ternary operator (`?:`) is properly implemented as right-associative.
- Error handling includes meaningful error messages and reporting of error locations.

### Lexer Notes

- The re2c lexer is implemented as a scanner that tokenizes the input string one token at a time.
- It handles variable names with both simple syntax (`$var`) and braced syntax (`${complex var}`).
- Special care is taken for case-insensitive handling of keywords and special literals.
- The lexer automatically skips whitespace and handles end-of-input conditions.

### Memory Management

- Uses Netdata's memory allocation patterns (`mallocz`, `freez`, etc.).
- Carefully tracks and frees memory in error conditions.
- Ensures proper cleanup on parse failure.