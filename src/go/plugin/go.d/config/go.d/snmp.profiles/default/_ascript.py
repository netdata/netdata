#!/usr/bin/env python3

import argparse
import os
import sys
from pathlib import Path
import google.generativeai as genai
from google.api_core import exceptions as google_exceptions

# --- Configuration ---
# You can change the default model here if needed
DEFAULT_GEMINI_MODEL = "gemini-2.0-flash" 

# --- Core Function ---
def process_yaml_with_gemini(input_yaml_path: Path, output_yaml_path: Path, model_name: str):
    """
    Reads an input YAML file, sends it to the Gemini API to add description
    and unit fields, and writes the result to an output YAML file.

    Args:
        input_yaml_path: Path to the input YAML file.
        output_yaml_path: Path where the processed YAML should be saved.
        model_name: The name of the Gemini model to use.
    """
    # 1. Check for API Key
    api_key = os.environ.get("GEMINI_API_KEY")
    if not api_key:
        print("Error: GEMINI_API_KEY environment variable not set.", file=sys.stderr)
        sys.exit(1)

    # 2. Read Input YAML File
    try:
        input_yaml_content = input_yaml_path.read_text(encoding='utf-8')
        print(f"Successfully read input file: {input_yaml_path}")
    except FileNotFoundError:
        print(f"Error: Input file not found: {input_yaml_path}", file=sys.stderr)
        sys.exit(1)
    except IOError as e:
        print(f"Error reading input file {input_yaml_path}: {e}", file=sys.stderr)
        sys.exit(1)

    # 3. Construct the Prompt
    #    Note: We explicitly ask the model to *only* output the YAML in backticks
    #    to make parsing the response easier.
    prompt = f"""Take the following input YAML file content and update it by adding a "description" and "unit" field for each metric entry under the `metrics:` section. If you are inside the `metadata` section, only add descriptions, not units.

- The "description" should be extracted from the preceding comment block (# comment) if available; use it verbatim. If the block does not describe what the metric is, but how it is fetched, or if no comment is present, generate a concise description based on the metric's `name` and `OID`. The description should explain what the metric is or represents. Do not explain if it was extracted or matched or originated from. **Use verbatim.**
- The "unit" should be inferred based on the metric's name, description, or common SNMP practices (e.g., %, seconds, bytes, packets, sessions, rate/s, etc.). The unit will be displayed in a monitoring chart. Choose UCUM case sensitive ("c/s") approved symbols and standard units. If no unit is applicable (e.g., for an index or state), add it as "TBD".
- Ensure the output is valid YAML.
- Preserve the original structure, indentation, and comments where possible, only adding the new fields.

Input YAML:
```yaml
{input_yaml_content}
```

Provide *only* the complete, updated YAML content enclosed within triple backticks (```yaml ... ```). Do not include any introductory text, explanations, or summaries outside the YAML block.
"""

    # 4. Configure and Call Gemini API
    try:
        print(f"Configuring Gemini client with model: {model_name}...")
        genai.configure(api_key=api_key)
        model = genai.GenerativeModel(model_name)

        print("Sending request to Gemini API...")
        # Use generate_content for potentially simpler handling if streaming isn't strictly needed
        # response = model.generate_content(prompt)
        # generated_text = response.text

        # Or stick with streaming if preferred / for long responses
        response_stream = model.generate_content(prompt, stream=True)
        generated_text = ""
        for chunk in response_stream:
            generated_text += chunk.text
            print(".", end="", flush=True) # Progress indicator
        print("\nReceived response from Gemini API.")

    except google_exceptions.GoogleAPIError as e:
        print(f"\nError calling Gemini API: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"\nAn unexpected error occurred during API interaction: {e}", file=sys.stderr)
        sys.exit(1)

    # 5. Extract YAML from Response
    #    Try to find the yaml block delimiters ```yaml ... ```
    start_marker = "```yaml\n"
    end_marker = "\n```"
    start_index = generated_text.find(start_marker)
    end_index = generated_text.rfind(end_marker) # Use rfind for the *last* occurrence

    if start_index != -1 and end_index != -1 and start_index < end_index:
        extracted_yaml = generated_text[start_index + len(start_marker):end_index].strip()
        print("Successfully extracted YAML block from response.")
    else:
        # Fallback: If markers aren't found, assume the whole response might be YAML,
        # but warn the user. This might happen if the model doesn't follow instructions perfectly.
        print("Warning: Could not find ```yaml delimiters in the response. Using the full response.", file=sys.stderr)
        extracted_yaml = generated_text.strip()
        # Basic check if it *looks* like YAML (starts with a common YAML key/structure)
        if not (extracted_yaml.startswith("extends:") or extracted_yaml.startswith("device:") or extracted_yaml.startswith("metrics:") or extracted_yaml.startswith("- MIB:")):
             print("Warning: The response doesn't seem to start like the expected YAML. Please verify the output file.", file=sys.stderr)


    # 6. Write Output YAML File
    try:
        # Ensure the output directory exists
        output_yaml_path.parent.mkdir(parents=True, exist_ok=True)
        output_yaml_path.write_text(extracted_yaml, encoding='utf-8')
        print(f"Successfully wrote processed YAML to: {output_yaml_path}")
    except IOError as e:
        print(f"Error writing output file {output_yaml_path}: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"An unexpected error occurred during file writing: {e}", file=sys.stderr)
        sys.exit(1)


# --- Main Execution ---
if __name__ == "__main__":
    parser = argparse.ArgumentParser(
        description="Update a YAML file by adding 'description' and 'unit' fields to metrics using the Gemini API.",
        formatter_class=argparse.ArgumentDefaultsHelpFormatter
    )
    parser.add_argument(
        "-i", "--input",
        required=True,
        type=Path,
        help="Path to the input YAML file."
    )
    parser.add_argument(
        "-o", "--output",
        required=True,
        type=Path,
        help="Path where the output YAML file will be saved."
    )
    parser.add_argument(
        "-m", "--model",
        default=DEFAULT_GEMINI_MODEL,
        help="Name of the Gemini model to use (e.g., 'gemini-1.5-pro-latest', 'gemini-pro')."
    )

    args = parser.parse_args()

    # Basic validation
    # if args.input == args.output:
    #     print("Error: Input and output file paths cannot be the same.", file=sys.stderr)
    #     sys.exit(1)

    process_yaml_with_gemini(args.input, args.output, args.model)
    print("Processing complete.")