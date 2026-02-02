#!/bin/bash
# Encrypt a credentials file with age (password-protected)
#
# Usage: ./encrypt-credentials.sh credentials.yaml credentials.enc
#
# Requires: age (https://github.com/FiloSottile/age)
#   Install: brew install age (macOS) or apt install age (Debian/Ubuntu)

set -e

if [ $# -lt 2 ]; then
    echo "Usage: $0 <input.yaml> <output.enc>"
    echo ""
    echo "Example:"
    echo "  $0 credentials.yaml credentials.enc"
    exit 1
fi

INPUT="$1"
OUTPUT="$2"

if [ ! -f "$INPUT" ]; then
    echo "Error: Input file '$INPUT' not found"
    exit 1
fi

if ! command -v age &> /dev/null; then
    echo "Error: 'age' command not found"
    echo "Install with: brew install age (macOS) or apt install age (Linux)"
    exit 1
fi

echo "Encrypting $INPUT -> $OUTPUT"
echo "You will be prompted for a password."
echo ""

age -p -o "$OUTPUT" "$INPUT"

echo ""
echo "Done! Encrypted file: $OUTPUT"
echo ""
echo "To use with credwrap-server:"
echo "  credwrap-server --config config.yaml --credentials $OUTPUT --encrypted"
