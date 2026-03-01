#! /usr/bin/env bash
set -e

TARGET="pico2-w"
OUTPUT_DIR="dist"

mkdir -p "$OUTPUT_DIR"

echo "========================================="
echo "Building Inky Library"
echo "========================================="
echo ""

echo "Building example program for $TARGET..."
echo ""
tinygo build -target=$TARGET -opt=2 -o "$OUTPUT_DIR/example.uf2" ./cmd/example
echo ""
echo "✓ Example built: $OUTPUT_DIR/example.uf2"
echo ""
echo "To flash:"
echo "  1. Hold BOOTSEL button while connecting Pico 2 W via USB"
echo "  2. Copy $OUTPUT_DIR/example.uf2 to the RPI-RP2 drive"
echo "  3. Device will reboot and run the example"

echo "========================================="
