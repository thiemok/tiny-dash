# tiny-dash

A dashboard display system for Raspberry Pi Pico 2 W with e-ink displays. The system consists of two main components that work together to display dynamic content on an e-ink screen.

## System Architecture

```
┌─────────────────┐         HTTP          ┌─────────────────┐
│   picoDevice    │ ◄─────────────────────►│   API Server    │
│  (Pico 2 W)     │   Image Data (packed)  │   (Go HTTP)     │
│                 │                        │                 │
│  - WiFi Client  │                        │  - Generates    │
│  - HTTP Client  │                        │    test images  │
│  - Display Mgmt │                        │  - Color bars   │
└────────┬────────┘                        │  - Timestamp    │
         │                                 └─────────────────┘
         ▼
  ┌─────────────┐
  │ Inky Display│
  │  (E-ink)    │
  └─────────────┘
```

## Components

### 1. picoDevice
TinyGo firmware running on Raspberry Pi Pico 2 W that:
- Connects to WiFi
- Fetches dashboard images from the API
- Displays content on e-ink displays
- Auto-detects display type via EEPROM
- Refreshes at configurable intervals

**Location**: `picoDevice/`  
**Documentation**: See [picoDevice/README.md](picoDevice/README.md)

### 2. API Server
Go HTTP server that:
- Generates dashboard images
- Creates test patterns (color bars + timestamp)
- Packs pixel data to match e-ink display format
- Accepts display parameters via query string

**Location**: `api/`

### 3. Inky Library
TinyGo library for Pimoroni Inky e-ink displays:
- Display drivers for various Inky models
- Auto-detection via EEPROM
- Framebuffer management with pixel packing
- Hardware abstraction for Pico 2 W

**Location**: `inky/`  
**Documentation**: See [inky/README.md](inky/README.md)

## Quick Start

### 1. Start the API Server

```bash
cd api
go run main.go
```

The API will start on port 8080.

### 2. Configure picoDevice

```bash
cd picoDevice
cp .env.example .env
# Edit .env with your WiFi credentials and API host
```

Example `.env`:
```bash
WIFI_SSID=YourWiFiNetwork
WIFI_PASSWORD=YourPassword
API_HOST=192.168.1.100  # Your computer's IP
API_PORT=8080
REFRESH_INTERVAL=60
```

### 3. Build and Flash picoDevice

```bash
cd picoDevice
bash tools/build.sh
```

Then flash `dist/picoDevice.uf2` to your Pico 2 W:
1. Hold BOOTSEL button while connecting USB
2. Copy `dist/picoDevice.uf2` to RPI-RP2 drive
3. Device will reboot and start

### 4. Monitor

```bash
tinygo monitor
```

You should see:
- Hardware initialization
- Display detection
- WiFi connection
- Image fetching
- Display updates every 60 seconds

## How It Works

1. **picoDevice** boots and auto-detects the e-ink display
2. Connects to WiFi using configured credentials
3. Sends HTTP GET request to API with display parameters:
   - Resolution (width x height)
   - Color depth (bits per pixel)
   - Supported colors
4. **API** generates a test image:
   - Vertical color bars (one per supported color)
   - Timestamp text overlay
   - Packs pixels matching display's framebuffer format
5. **picoDevice** receives packed pixel data
6. Copies data to display framebuffer
7. Updates e-ink display (takes 30-40 seconds)
8. Waits for refresh interval
9. Repeats from step 3

## Data Format

The API returns raw packed pixel data that matches the e-ink display's framebuffer format:
- Pixels are packed based on color depth
- For 3-bit color: 2 pixels per byte (8/3 = 2.67, rounded down)
- Pixel packing uses LSB-first ordering
- Binary data is sent as `application/octet-stream`

## Development

### Prerequisites

- **For picoDevice**: TinyGo 0.30+
- **For API**: Go 1.21+
- **Build Tools**: Nx (for workspace management)

### Workspace Structure

```
tiny-dash/
├── picoDevice/        # Pico 2 W firmware
├── api/               # HTTP image server
├── inky/              # E-ink display library
├── nx.json            # Nx workspace config
└── package.json       # Workspace dependencies
```

### Nx Commands

```bash
# Build picoDevice firmware
nx build picoDevice

# Upload to device
nx upload picoDevice

# Monitor serial output
nx monitor picoDevice

# Build inky library
nx build inky
```

## Supported Displays

The system supports all Pimoroni Inky displays with EEPROM, including:
- Inky Impression 7.3" (7-color, 800x480)
- Inky Impression 5.7" (7-color, 600x448)
- Inky Impression 4" (7-color, 640x400)
- Inky wHAT (3-color, 400x300)
- Inky pHAT (3-color, 212x104)

Display type is auto-detected at runtime via EEPROM.

## Next Steps

This implementation provides the basic architecture. Future enhancements could include:

1. **Dynamic Content**: Replace test pattern with real dashboard data
2. **Multiple Dashboards**: Support different dashboard layouts
3. **Web UI**: Configuration interface for API
4. **OTA Updates**: Update firmware over WiFi
5. **Data Sources**: Integration with metrics, calendars, weather, etc.

## License

See LICENSE file for details.

## Contributing

Contributions welcome! Please see GUIDELINES.md for development standards.
