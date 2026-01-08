#!/bin/bash
set -e

# Note: Using pico-w target as Pico 2 W is backward compatible
# Once TinyGo adds pico-2-w target, update this
TARGET="pico-w"
OUTPUT_DIR="dist"
OUTPUT_FILE="$OUTPUT_DIR/device.uf2"

mkdir -p "$OUTPUT_DIR"

echo "Building device firmware for Raspberry Pi Pico 2 W (using pico-w target)..."

tinygo build -target=$TARGET -opt=2 -o $OUTPUT_FILE ./cmd/device

echo "✓ Device firmware built: $OUTPUT_FILE"
echo ""
echo "To flash:"
echo "  1. Hold BOOTSEL button while connecting Pico 2 W via USB"
echo "  2. Copy $OUTPUT_FILE to the RPI-RP2 drive"
echo "  3. Device will reboot automatically"
