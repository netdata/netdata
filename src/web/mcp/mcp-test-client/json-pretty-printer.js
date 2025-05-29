/**
 * JSON Pretty Printer with Syntax Highlighting
 * Features:
 * - Visualizes newlines in strings
 * - Detects and formats nested JSON in string values
 * - Syntax highlighting with colors
 * - Handles large JSON structures efficiently
 */

class JSONPrettyPrinter {
    constructor(options = {}) {
        this.options = {
            indent: 2,
            maxDepth: 100,
            maxStringLength: 1048576, // 1 MiB
            visualizeNewlines: true,
            detectNestedJSON: true,
            syntaxHighlight: true,
            colors: {
                key: '#881391',
                string: '#0B7500',
                number: '#1750EB',
                boolean: '#0033B3',
                null: '#666666',
                punctuation: '#999999',
                newline: '#0088cc',
                background: {
                    newline: '#e6f3ff'
                }
            },
            ...options
        };
    }

    /**
     * Pretty print JSON with all features
     */
    prettyPrint(obj, indent = 0) {
        if (indent > this.options.maxDepth) {
            console.warn(`Max depth (${this.options.maxDepth}) reached at indent level ${indent}`);
            return this._colorize('"[Max depth reached]"', 'string');
        }

        if (obj === null) {
            return this._colorize('null', 'null');
        }

        const type = typeof obj;
        
        switch (type) {
            case 'boolean':
                return this._colorize(obj.toString(), 'boolean');
                
            case 'number':
                return this._colorize(obj.toString(), 'number');
                
            case 'string':
                return this._formatString(obj, indent);
                
            case 'object':
                if (Array.isArray(obj)) {
                    return this._formatArray(obj, indent);
                } else {
                    return this._formatObject(obj, indent);
                }
                
            default:
                return this._colorize(JSON.stringify(obj), 'string');
        }
    }

    /**
     * Format a string value with newline visualization and nested JSON detection
     */
    _formatString(str, indent) {
        // Check if string might contain JSON
        if (this.options.detectNestedJSON && this._looksLikeJSON(str)) {
            console.log(`Detected nested JSON at indent ${indent}, string length: ${str.length}`);
            try {
                // First, try to parse as-is
                const parsed = JSON.parse(str);
                console.log(`Successfully parsed nested JSON, formatting at indent ${indent + 1}`);
                // If it's valid JSON, format it as nested JSON
                const formatted = this.prettyPrint(parsed, indent + 1);
                // Wrap in quotes but show it's parsed JSON
                return this._colorize('"', 'string') + 
                       this._colorize('[JSON] ', 'newline') +
                       formatted + 
                       this._colorize('"', 'string');
            } catch (e) {
                // If direct parsing fails, try unescaping first
                console.log('Direct parse failed, trying to unescape first');
                console.log('First 200 chars of string:', str.substring(0, 200));
                try {
                    // Unescape the string (handle \" -> ")
                    const unescaped = str.replace(/\\"/g, '"').replace(/\\\\/g, '\\');
                    console.log('First 200 chars after unescape:', unescaped.substring(0, 200));
                    const parsed = JSON.parse(unescaped);
                    console.log(`Successfully parsed unescaped JSON, formatting at indent ${indent + 1}`);
                    // If it's valid JSON, format it as nested JSON
                    const formatted = this.prettyPrint(parsed, indent + 1);
                    // Wrap in quotes but show it's parsed JSON
                    return this._colorize('"', 'string') + 
                           this._colorize('[JSON] ', 'newline') +
                           formatted + 
                           this._colorize('"', 'string');
                } catch (e2) {
                    console.error('Failed to parse even after unescaping:', e2.message);
                    // Not valid JSON, continue with normal string formatting
                }
            }
        }

        // Escape special characters
        let escaped = str
            .replace(/\\/g, '\\\\')
            .replace(/"/g, '\\"')
            .replace(/\t/g, '\\t')
            .replace(/\r/g, '\\r');

        // Visualize newlines if enabled
        if (this.options.visualizeNewlines && escaped.includes('\\n')) {
            // Split by newlines but keep them
            const parts = escaped.split(/(\\n)/);
            const formatted = parts.map((part, i) => {
                if (part === '\\n') {
                    return `<span class="json-newline-wrapper">` +
                           `<span class="json-newline">\\n</span>` +
                           `</span>`;
                }
                return this._escapeHtml(part);
            }).join('');
            
            return this._colorize('"', 'string') + 
                   `<span class="json-string">` + formatted + `</span>` +
                   this._colorize('"', 'string');
        }

        // Regular string
        return this._colorize('"' + this._escapeHtml(escaped) + '"', 'string');
    }

    /**
     * Format an object
     */
    _formatObject(obj, indent) {
        const keys = Object.keys(obj);
        if (keys.length === 0) {
            return this._colorize('{}', 'punctuation');
        }

        const spaces = ' '.repeat(indent * this.options.indent);
        const innerSpaces = ' '.repeat((indent + 1) * this.options.indent);
        
        let result = this._colorize('{', 'punctuation') + '\n';
        
        keys.forEach((key, index) => {
            result += innerSpaces;
            result += this._colorize('"' + this._escapeHtml(key) + '"', 'key');
            result += this._colorize(': ', 'punctuation');
            result += this.prettyPrint(obj[key], indent + 1);
            
            if (index < keys.length - 1) {
                result += this._colorize(',', 'punctuation');
            }
            result += '\n';
        });
        
        result += spaces + this._colorize('}', 'punctuation');
        return result;
    }

    /**
     * Format an array
     */
    _formatArray(arr, indent) {
        if (arr.length === 0) {
            return this._colorize('[]', 'punctuation');
        }

        const spaces = ' '.repeat(indent * this.options.indent);
        const innerSpaces = ' '.repeat((indent + 1) * this.options.indent);
        
        let result = this._colorize('[', 'punctuation') + '\n';
        
        arr.forEach((item, index) => {
            result += innerSpaces;
            result += this.prettyPrint(item, indent + 1);
            
            if (index < arr.length - 1) {
                result += this._colorize(',', 'punctuation');
            }
            result += '\n';
        });
        
        result += spaces + this._colorize(']', 'punctuation');
        return result;
    }

    /**
     * Check if a string looks like it might contain JSON
     */
    _looksLikeJSON(str) {
        if (str.length > this.options.maxStringLength) {
            return false;
        }
        
        const trimmed = str.trim();
        return (trimmed.startsWith('{') && trimmed.endsWith('}')) ||
               (trimmed.startsWith('[') && trimmed.endsWith(']'));
    }

    /**
     * Apply color to text based on type
     */
    _colorize(text, type) {
        if (!this.options.syntaxHighlight) {
            return text;
        }

        const color = this.options.colors[type];
        if (!color) {
            return text;
        }

        return `<span class="json-${type}" style="color: ${color}">${text}</span>`;
    }

    /**
     * Escape HTML special characters
     */
    _escapeHtml(str) {
        const div = document.createElement('div');
        div.textContent = str;
        return div.innerHTML;
    }

    /**
     * Convert to HTML with proper styling
     */
    toHTML(obj) {
        const content = this.prettyPrint(obj);
        
        // Add CSS for newline visualization
        const style = `
            <style>
                .json-pretty {
                    font-family: 'Courier New', Consolas, monospace;
                    font-size: 13px;
                    line-height: 1.5;
                    white-space: pre;
                    overflow-x: auto;
                }
                .json-newline-wrapper {
                    display: inline;
                    position: relative;
                }
                .json-newline {
                    background-color: ${this.options.colors.background.newline};
                    color: ${this.options.colors.newline};
                    padding: 1px 3px;
                    border-radius: 3px;
                    font-size: 11px;
                    font-weight: bold;
                    margin: 0 1px;
                }
                .json-string {
                    color: ${this.options.colors.string};
                }
                .json-key {
                    color: ${this.options.colors.key};
                }
                .json-number {
                    color: ${this.options.colors.number};
                }
                .json-boolean {
                    color: ${this.options.colors.boolean};
                }
                .json-null {
                    color: ${this.options.colors.null};
                }
                .json-punctuation {
                    color: ${this.options.colors.punctuation};
                }
            </style>
        `;
        
        return style + '<div class="json-pretty">' + content + '</div>';
    }

    /**
     * Convert to plain HTML without wrapper (for embedding)
     */
    toHTMLContent(obj) {
        return this.prettyPrint(obj);
    }

    /**
     * Format for console output (no HTML)
     */
    toConsole(obj) {
        const oldHighlight = this.options.syntaxHighlight;
        this.options.syntaxHighlight = false;
        const result = this.prettyPrint(obj);
        this.options.syntaxHighlight = oldHighlight;
        return result;
    }
}

// Export for use
if (typeof module !== 'undefined' && module.exports) {
    module.exports = JSONPrettyPrinter;
}