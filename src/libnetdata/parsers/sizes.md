## Data Sizes in Netdata

Netdata provides a flexible system for specifying and formatting data sizes, used in various configurations and operations such as disk space management, and memory usage. This system allows users to specify data sizes in a human-readable format using multiple units from bytes to terabytes, supporting both binary (base-2) and decimal (base-10) standards. All units are UCUM-based for consistency and clarity.

### Supported Size Units

The following table lists all supported units and their corresponding values:

| Symbol | Description |   Value   |  Base   | Formatter |
|:------:|:-----------:|:---------:|:-------:|:---------:|
|  `B`   |    Bytes    |   `1B`    |    -    |  **Yes**  |
|  `k`   |  Kilobytes  |  `1000B`  | Base-10 |    No     |
|  `K`   |  Kilobytes  |  `1000B`  | Base-10 |    No     |
|  `KB`  |  Kilobytes  |  `1000B`  | Base-10 |    No     |
| `KiB`  |  Kibibytes  |  `1024B`  | Base-2  |  **Yes**  |
|  `M`   |  Megabytes  |  `1000K`  | Base-10 |    No     |
|  `MB`  |  Megabytes  |  `1000K`  | Base-10 |    No     |
| `MiB`  |  Mebibytes  | `1024KiB` | Base-2  |  **Yes**  |
|  `G`   |  Gigabytes  |  `1000M`  | Base-10 |    No     |
|  `GB`  |  Gigabytes  |  `1000M`  | Base-10 |    No     |
| `GiB`  |  Gibibytes  | `1024MiB` | Base-2  |  **Yes**  |
|  `T`   |  Terabytes  |  `1000G`  | Base-10 |    No     |
|  `TB`  |  Terabytes  |  `1000G`  | Base-10 |    No     |
| `TiB`  |  Tebibytes  | `1024GiB` | Base-2  |  **Yes**  |
|  `P`   |  Petabytes  |  `1000T`  | Base-10 |    No     |
|  `PB`  |  Petabytes  |  `1000T`  | Base-10 |    No     |
| `PiB`  |  Pebibytes  | `1024TiB` | Base-2  |  **Yes**  |

### Size Expression Format

Netdata allows users to express sizes using a number followed by a unit, such as `500MiB` (500 Mebibytes), `1GB` (1 Gigabyte), or `256K` (256 Kilobytes).

- **Case Sensitivity**: Note that the parsing of units is case-sensitive.

### Size Formatting

Netdata formats a numeric size value (in bytes) into a human-readable string with an appropriate unit. The formatter's goal is to select the largest unit that can represent the size exactly, using up to two fractional digits. If two fractional digits are not enough to precisely represent the byte count, the formatter will use a smaller unit until it can accurately express the size, eventually falling back to bytes (`B`) if necessary.

When formatting, Netdata prefers Base-2 units (`KiB`, `MiB`, `GiB`, etc.).

- **Examples of Size Formatting**:
    - **10,485,760 bytes** is formatted as `10MiB` (10 Mebibytes).
    - **1,024 bytes** is formatted as `1KiB` (1 Kibibyte).
    - **1,500 bytes** remains formatted as `1500B` because it cannot be precisely represented in `KiB` or any larger unit using up to two fractional digits.

### Example Size Expressions

Here are some examples of valid size expressions:

1. `1024B`: 1024 bytes.
2. `1KiB`: 1024 bytes.
3. `5MiB`: 5 mebibytes (5 * 1024 * 1024 bytes).
