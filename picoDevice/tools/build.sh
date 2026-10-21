#! /usr/bin/env bash
set -e

TARGET="pico2-w"
OUTPUT_DIR="dist"
OUTPUT_FILE="$OUTPUT_DIR/picoDevice.uf2"

# Both modes use these flags, so `tools/build.sh flash` (the upload target)
# flashes the same firmware that a build produces.
FLAGS=(-target=$TARGET -scheduler=tasks -stack-size=8kb -opt=2 ${PICO_BUILD_TAGS:+-tags=$PICO_BUILD_TAGS})

if [ "$1" = flash ]; then
  echo "Flashing picoDevice firmware to Pico 2 W${PICO_BUILD_TAGS:+ (tags: $PICO_BUILD_TAGS)}..."
  exec tinygo flash "${FLAGS[@]}" ./cmd/picoDevice
fi

mkdir -p "$OUTPUT_DIR"

echo "Building picoDevice firmware for Raspberry Pi Pico 2 W${PICO_BUILD_TAGS:+ (tags: $PICO_BUILD_TAGS)}..."

tinygo build "${FLAGS[@]}" -o $OUTPUT_FILE ./cmd/picoDevice

echo "✓ picoDevice firmware built: $OUTPUT_FILE"
echo ""
echo "Configuration:"
echo "  WiFi SSID: $WIFI_SSID"
echo "  API Host: $API_HOST:$API_PORT"
echo "  Refresh Interval: ${REFRESH_INTERVAL}s"
echo ""
echo "To flash:"
echo "  1. Hold BOOTSEL button while connecting Pico 2 W via USB"
echo "  2. Copy $OUTPUT_FILE to the RPI-RP2 drive"
echo "  3. Device will reboot automatically"
