# re2c/lemon Expression Parser for Netdata

This is an experimental implementation of a parser for Netdata's expression language using:
- re2c for lexical analysis (tokenization)
- lemon for parsing

The implementation integrates with Netdata's existing EVAL_NODE structure, making it compatible with the current expression evaluator.

## Requirements

To build this parser, you need:

- re2c (for generating the lexer)
- lemon (for generating the parser)

On Manjaro/Arch Linux, you can install these with:

```bash
sudo pacman -S re2c lemon
```

## Generating the Parser Files

To generate the lexer and parser, run:

```bash
make
```

This will generate:
1. lexer.c from lexer.re (using re2c)
2. parser.c and parser.h from parser.y (using lemon)

## Integration with Netdata

The integration with Netdata is already prepared. To use the re2c/lemon parser:

1. Generate the parser files:
   ```bash
   cd /home/costa/src/netdata-ktsaou.git/src/libnetdata/eval/re2c_lemon
   make
   ```

   If you encounter issues downloading lempar.c, you can manually copy it from a system installation:
   ```bash
   # On Manjaro/Arch, the file is typically at:
   cp /usr/share/lemon/lempar.c .
   # Then run make again
   make
   ```

2. The generated files are already in the correct location. Make sure lexer.c, parser.c, and parser.h exist:
   ```bash
   ls -la lexer.c parser.c parser.h
   ```

3. Uncomment the following line in `src/libnetdata/eval/eval-internal.h`:
   ```c
   // #define USE_RE2C_LEMON_PARSER
   ```

4. Rebuild Netdata with the new parser
   ```bash
   cd /home/costa/src/netdata-ktsaou.git
   ./netdata-installer.sh --disable-cloud
   ```

5. Run the unit tests to verify the parser:
   ```bash
   ./tests/run-unit-tests.sh eval
   ```

To switch back to the original parser, just comment out the `USE_RE2C_LEMON_PARSER` define and rebuild.

## Comparing Parser Behaviors

You can compare the behavior of both parsers by:

1. Running the tests with the original parser (default)
2. Recording the test results
3. Uncommenting the `USE_RE2C_LEMON_PARSER` define
4. Running the tests again with the re2c/lemon parser
5. Comparing the results

The unit tests will display which parser is being used in the output, making it easy to spot differences.

## Benefits over the Current Recursive Descent Parser

1. **Cleaner Separation**: The lexer and parser are clearly separated
2. **Better Error Handling**: The parser provides more precise error positions
3. **Easier to Maintain**: Grammar changes only require modifying the lemon grammar
4. **Performance**: re2c generates highly optimized lexers

## Advantages over the bison/flex Implementation

1. **EVAL_NODE Integration**: Uses Netdata's existing EVAL_NODE structure
2. **Simpler**: re2c is more straightforward than flex
3. **More Compact**: lemon generates smaller parsers than bison
4. **Thread Safety**: Both re2c and lemon are designed for thread safety

## Variable Name Rules

The parser supports flexible variable name formats:

1. **Simple variables**: `$variable_name`
   - Can start with letters, numbers, or underscores
   - Can contain letters, numbers, underscores, and dots
   - Examples: `$var1`, `$1var`, `$var.name`, `$123`, `$var.1`

2. **Braced variables**: `${variable name}`
   - Can contain any characters except `}` and null terminator
   - Useful for variable names with spaces or special characters
   - Examples: `${complex name}`, `${var-with-hyphens}`, `${1.2.3}`

## Future Improvements

1. Add more operator types
2. Improve error reporting
3. Add syntax tree validation
4. Add more function support
5. Performance optimizations