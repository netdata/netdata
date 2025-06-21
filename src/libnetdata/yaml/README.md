# Netdata YAML Parser/Generator Module

This module provides YAML parsing and generation capabilities for Netdata, with seamless conversion to/from json-c objects. It uses the libyaml library for parsing and generation, supporting the YAML subset that matches JSON 100%.

## Features

### Supported Operations

1. **Parse YAML from various sources:**
   - String buffer (`yaml_parse_string`)
   - File by filename (`yaml_parse_filename`)
   - File descriptor (`yaml_parse_fd`)

2. **Generate YAML to various destinations:**
   - BUFFER (`yaml_generate_to_buffer`)
   - File by filename (`yaml_generate_to_filename`)
   - File descriptor (`yaml_generate_to_fd`)

3. **Data types supported:**
   - Null values
   - Booleans
   - Numbers (integers and floating-point)
   - Strings
   - Arrays
   - Objects (maps)

## Supported YAML Features

### 1. Basic Data Types

```yaml
# Null values (all case-insensitive)
null_value1: null
null_value2: Null
null_value3: NULL
null_value4: ~

# Booleans (all case-insensitive)
bool_true: true    # also: True, TRUE, yes, Yes, YES, on, On, ON
bool_false: false  # also: False, FALSE, no, No, NO, off, Off, OFF

# Numbers
integer: 42
negative: -123
float: 3.14159
scientific: 1.23e-10
```

### 2. Advanced Number Formats

```yaml
# Hexadecimal (parsed as integers)
hex_lower: 0x1a
hex_upper: 0XFF
hex_large: 0xDEADBEEF

# Octal (YAML 1.2 style)
octal_new: 0o755
octal_caps: 0O644

# Binary
binary_lower: 0b1010
binary_upper: 0B11111111

# Numbers with underscores (YAML 1.2)
readable_int: 1_000_000
readable_float: 3.141_592_653
readable_hex: 0x1_A_B_C
readable_binary: 0b1010_1010
```

### 3. Strings

```yaml
# Plain strings
plain: Hello World

# Single quoted (literal strings)
single: 'This is a single quoted string'
single_escape: 'Can''t escape much in single quotes'

# Double quoted (with escape sequences)
double: "Hello\nWorld"
escaped: "Tab:\t Quote:\" Backslash:\\"

# Special strings that need quoting
quoted_null: "null"     # Without quotes would be null value
quoted_bool: "true"     # Without quotes would be boolean
quoted_number: "123"    # Without quotes would be number
```

### 4. Collections

```yaml
# Arrays
simple_array: [1, 2, 3]
block_array:
  - item1
  - item2
  - nested: value

# Objects/Maps
simple_object: {key1: value1, key2: value2}
block_object:
  name: John Doe
  age: 30
  address:
    street: 123 Main St
    city: Anytown
```

### 5. Multiline Strings

```yaml
# Literal block scalar (preserves newlines and spacing)
literal: |
  Line 1
  Line 2
    Indented line

# Folded block scalar (folds newlines to spaces)
folded: >
  This is a long
  paragraph that will
  be folded into a
  single line.
```

## Round-Trip Consistency

The module ensures round-trip consistency for most data types:

```c
// Original JSON
{"number": 1.0, "text": "hello", "flag": true}

// Generated YAML
number: 1.0
text: hello
flag: true

// Parsed back to JSON
{"number": 1.0, "text": "hello", "flag": true}
```

Special handling for floating-point numbers ensures that `1.0` remains `1.0` and doesn't become `1`.

## Limitations Due to libyaml

### 1. Unsupported Escape Sequences

**Octal escapes** are not supported by libyaml:
```yaml
# This will fail to parse
invalid: "\101"  # Octal escape for 'A'

# Workaround: use hex or unicode escapes
valid: "\x41"    # Hex escape for 'A'
valid: "\u0041"  # Unicode escape for 'A'
```

### 2. Null Bytes in Strings

libyaml has issues with embedded null bytes:
```yaml
# This may not work correctly
with_null: "before\x00after"

# The null byte may terminate the string early
# or cause unexpected behavior
```

### 3. Single-Quoted Literal Newlines

In single-quoted strings, literal newlines are converted to spaces by libyaml:
```yaml
# Input
single: 'line1
line2'

# libyaml parses this as
single: 'line1 line2'

# To preserve newlines, use double quotes or block scalars
double: "line1\nline2"
literal: |
  line1
  line2
```

### 4. Block Scalar Indentation

Complex indentation in block scalars may not be preserved exactly:
```yaml
# Deep indentation might be normalized
literal: |
    deeply
      indented
    text

# May lose some leading spaces depending on context
```

### 5. Invalid Syntax Detection

Some invalid YAML syntax may be accepted by libyaml:
```yaml
# This should be invalid but might parse
- item
- - invalid nesting

# Proper nesting would be
- item
- 
  - valid nesting
```

## Usage Examples

### Parsing YAML

```c
// Parse from string
BUFFER *error = buffer_create(0, NULL);
const char *yaml = "name: Netdata\nversion: 1.0\n";
struct json_object *json = yaml_parse_string(yaml, error);

if (buffer_strlen(error) > 0) {
    fprintf(stderr, "Parse error: %s\n", buffer_tostring(error));
} else {
    // Use json object (note: NULL is valid for YAML null)
    if (json) {
        printf("Parsed: %s\n", json_object_to_json_string(json));
        json_object_put(json);
    } else {
        printf("Parsed YAML null value\n");
    }
}

buffer_free(error);
```

### Generating YAML

```c
// Create JSON object
struct json_object *root = json_object_new_object();
json_object_object_add(root, "name", json_object_new_string("Netdata"));
json_object_object_add(root, "version", json_object_new_double(1.0));

// Generate YAML
BUFFER *output = buffer_create(0, NULL);
BUFFER *error = buffer_create(0, NULL);

if (yaml_generate_to_buffer(output, root, error)) {
    printf("Generated YAML:\n%s", buffer_tostring(output));
} else {
    fprintf(stderr, "Generation error: %s\n", buffer_tostring(error));
}

json_object_put(root);
buffer_free(output);
buffer_free(error);
```

## Testing

The module includes comprehensive unit tests covering:

1. **Basic tests** (`yaml-unittest.c`):
   - All data types
   - File operations
   - Error handling
   - Round-trip conversion

2. **Comprehensive tests** (`yaml-comprehensive-unittest.c`):
   - All YAML string styles
   - All number formats
   - Edge cases
   - Unicode and special characters
   - Multiline strings

3. **Stress testing** (`yaml-comprehensive-test.c`):
   - Random YAML generation
   - Large documents (up to 1MB)
   - Deep nesting
   - Memory leak detection

## Implementation Notes

1. **Null Handling**: JSON null is represented as C NULL pointer in json-c, which is properly handled in both parsing and generation.

2. **Number Parsing**: The parser attempts to identify number types in this order:
   - Hexadecimal (0x/0X prefix)
   - Octal (0o/0O prefix)
   - Binary (0b/0B prefix)
   - Integer (decimal)
   - Floating-point

3. **String Quoting**: Strings are automatically quoted during generation if they:
   - Could be misinterpreted as null, boolean, or number
   - Contain special characters
   - Have leading/trailing spaces
   - Contain newlines

4. **Memory Management**: All allocated memory is properly freed. The module has been tested with AddressSanitizer and LeakSanitizer.

## Thread Safety

The module is thread-safe as long as:
- Different threads use different parser/emitter instances
- json-c objects are not shared between threads without proper synchronization
- BUFFER objects are not shared between threads without proper synchronization