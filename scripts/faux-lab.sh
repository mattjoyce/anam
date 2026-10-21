#!/bin/bash
# Script to host a faux record in the lab as mentioned in card 210

set -euo pipefail

usage() {
    echo "Usage: $0 <directory> <port>"
    echo "  directory: directory containing the faux record or where to create it"
    echo "  port: port to listen on (default: 18790)"
    echo
    echo "Example:"
    echo "  $0 ~/admin/nonprod/anam-faux 18790"
    exit 1
}

# Parse arguments
if [[ $# -lt 1 ]]; then
    usage
fi

DIR="$1"
PORT="${2:-18790}"

# Ensure directory exists
mkdir -p "$DIR"

# Check if we need to generate a record
LEDGER_FILE="$DIR/ledger.db"
if [[ ! -f "$LEDGER_FILE ]]; then
    echo "No record found in $DIR, generating one..."
    # Generate a realistic faux record
    ./anam-bench gen --shape realistic --dir "$DIR" --seed 42
    if [[ $? -ne 0 ]]; then
        echo "Failed to generate faux record"
        exit 1
    fi
fi

# Create a token file
TOKEN_FILE="$DIR/token"
TOKEN="faux-token-$(date +%s)"
echo "$TOKEN" > "$TOKEN_FILE"
chmod 600 "$TOKEN_FILE"

echo "Starting faux daemon on port $PORT serving record from $DIR"
echo "Token: $TOKEN"
echo "Press Ctrl+C to stop"

# Start the daemon
./anam daemon --dir "$DIR" --listen "127.0.0.1:$PORT" --token-file "$TOKEN_FILE"