# tiny-dash

An e-ink dashboard system for displaying information on a 7.3" Inky Impression Spectra 6 display.

## Overview

tiny-dash displays a rotating set of dashboards (weather, calendar, public transport schedules) on an e-ink display powered by an embedded device. Dashboards are rendered as HTML on a server, converted to 6-color bitmap images, and displayed on the e-ink screen.

**Key Features:**
- Server-side rendered HTML dashboards
- 6-color e-ink display support (800x480px)
- Button-controlled dashboard cycling
- Network-enabled embedded device
- Resilient caching and error handling
- Built with Go/TinyGo

## Architecture

```
┌─────────────┐
│  Embedded   │  Button press
│   Device    │──────────────┐
│  (TinyGo)   │              │
└──────┬──────┘              │
       │                     ▼
       │ HTTP         ┌──────────────┐
       │◄─────────────┤   Renderer   │
       │  Image       │   Service    │
       │  (BMP)       │     (Go)     │
       │              └──────┬───────┘
       │                     │
       │                     │ Renders
       ▼                     ▼
┌─────────────┐      ┌──────────────┐
│   E-ink     │      │  Dashboards  │
│   Display   │      │ (HTML/CSS)   │
│ Spectra 6   │      └──────────────┘
└─────────────┘
```

## Project Structure

```
tiny-dash/
├── renderer/            # Renderer HTTP service (Go module)
│   ├── go.mod
│   ├── project.json     # Nx configuration
│   ├── cmd/renderer/
│   └── dist/
├── dashboards/          # Dashboard implementations (Go module)
│   ├── go.mod
│   ├── project.json
│   ├── pkg/dashboards/
│   └── dist/
├── device/              # Embedded device controller (TinyGo module)
│   ├── go.mod
│   ├── project.json
│   ├── cmd/device/
│   └── dist/
├── eink/                # E-ink display driver (TinyGo module)
│   ├── go.mod
│   ├── project.json
│   ├── pkg/eink/
│   └── dist/
├── tools/               # Build scripts for TinyGo modules
│   ├── build-device.sh
│   ├── build-eink.sh
│   ├── flash-device.sh
│   └── test-tinygo.sh
├── tasks/               # Detailed task files for each package
│   ├── renderer.md
│   ├── device.md
│   ├── eink.md
│   ├── dashboards.md
│   └── tools.md
├── nx.json              # Nx workspace configuration
├── package.json         # Node dependencies (Nx, @nx-go/nx-go)
├── GUIDELINES.md        # Implementation guidelines
└── README.md            # This file
```

**Module Structure:**
- Each component is a separate Go module with its own `go.mod`
- Standard Go modules (`renderer`, `dashboards`) use `@nx-go/nx-go` plugin
- TinyGo modules (`device`, `eink`) use shell scripts via `nx:run-commands`
- Inter-module dependencies managed with `replace` directives

## Components

### Renderer Service (renderer/)
HTTP service that renders HTML dashboards to 6-color bitmap images suitable for the e-ink display.

**Key Features:**
- Headless browser rendering with chromedp
- RGB to 6-color Spectra palette conversion
- Image caching for performance and resilience
- RESTful API for image retrieval

[→ Detailed task file: tasks/renderer.md](tasks/renderer.md)

### Embedded Device Controller (device/)
TinyGo application running on the embedded device, managing button input, network communication, and display control.

**Key Features:**
- GPIO button handling with debouncing
- HTTP client for fetching dashboard images
- E-ink display integration
- Network resilience and caching

[→ Detailed task file: tasks/device.md](tasks/device.md)

### E-ink Display Driver (eink/)
Go/TinyGo driver for the Inky Impression Spectra 6 display, ported from Pimoroni's Python library.

**Key Features:**
- SPI communication with UC8159 controller
- 6-color palette support
- Display initialization and refresh
- Hardware abstraction for testing

[→ Detailed task file: tasks/eink.md](tasks/eink.md)

### Dashboards (dashboards/)
Server-side rendered dashboard modules for weather, calendar, and public transport information.

**Key Features:**
- Weather dashboard (current conditions, forecast)
- Calendar dashboard (upcoming events)
- Public transport dashboard (real-time departures)
- Template-based HTML generation
- API integration with caching

[→ Detailed task file: tasks/dashboards.md](tasks/dashboards.md)

### Build Tools (tools/)
Unified build system using Make, development utilities, and deployment automation.

**Key Features:**
- Single command builds for all packages
- TinyGo cross-compilation support
- Hot-reload development server
- Deployment and flashing scripts

[→ Detailed task file: tasks/tools.md](tasks/tools.md)

## Hardware Requirements

- **Display**: Inky Impression 7.3" (Spectra 6, 800x480px, 6-color)
- **Microcontroller**: TBD (Raspberry Pi Pico W, ESP32, or similar)
  - Minimum 512KB RAM (preferably 1MB+)
  - WiFi connectivity
  - SPI and GPIO support
- **Button**: Single push button for cycling dashboards
- **Power**: USB or battery (power requirements TBD)

## Software Requirements

- **Go**: 1.21 or later
- **TinyGo**: 0.30 or later  
- **Node.js**: 18+ (for Nx)
- **Nx**: 17+ (installed via npm)
- **@nx-go/nx-go**: Nx plugin for Go
- **Chrome/Chromium**: For renderer (headless browser)
- **Target hardware toolchain**: picotool (Pico), esptool (ESP32), etc.

## Implementation Guidelines

See [GUIDELINES.md](GUIDELINES.md) for detailed implementation guidelines including:

1. Server-side rendering requirements
2. Color palette constraints
3. Network resilience strategies
4. E-ink optimization best practices
5. Go/TinyGo constraints
6. Build system consistency
7. Image format specifications

## Getting Started

### Initial Setup

The project is currently in the planning phase. Task files have been created for each package to guide future implementation.

**Next Steps:**
1. Review task files in `tasks/` directory
2. Set up development environment (Go, TinyGo, Make)
3. Choose target hardware platform
4. Begin implementation following task files

### Building (Future)

Once implemented, the build process will use Nx:

```bash
# Install dependencies
npm install

# Build everything
nx run-many --target=build --all

# Build specific module
nx build renderer
nx build device

# Run tests
nx run-many --target=test --all
nx test renderer

# Development server
nx serve renderer

# Flash device firmware
nx flash device

# View dependency graph
nx graph

# Build affected modules only
nx affected:build
```

See [tasks/tools.md](tasks/tools.md) for complete build system documentation.

## Development Status

**Current Phase:** Planning

**Completed:**
- ✅ Project architecture defined (multi-module monorepo)
- ✅ Implementation guidelines documented
- ✅ Task files created for all packages
- ✅ Build system architecture designed (Nx-based)

**Next Steps:**
- [ ] Initialize Nx workspace and install dependencies
- [ ] Create go.mod for each module
- [ ] Set up Nx project.json configurations
- [ ] Create build scripts for TinyGo modules
- [ ] Begin renderer service implementation
- [ ] Port e-ink driver from Python
- [ ] Develop initial dashboard templates
- [ ] Build device firmware
- [ ] Integration testing

## Task Files

Detailed task files for each package are available in the `tasks/` directory:

- **[tasks/renderer.md](tasks/renderer.md)** - Renderer service planning
- **[tasks/device.md](tasks/device.md)** - Device controller planning
- **[tasks/eink.md](tasks/eink.md)** - E-ink driver planning
- **[tasks/dashboards.md](tasks/dashboards.md)** - Dashboards package planning
- **[tasks/tools.md](tasks/tools.md)** - Build tools planning

Each task file includes:
- Package overview and responsibilities
- High-level architecture
- Key components and implementation details
- Technology choices
- Implementation approach (phases)
- Testing strategy
- Success criteria
- Open questions for future planning

## Contributing

This is currently a personal project in the planning phase. Contribution guidelines will be added once the initial implementation is complete.

## License

TBD

## Acknowledgments

- **Pimoroni** for the Inky Impression display and Python library
- **TinyGo** project for enabling Go on embedded systems

---

**Project Version:** 0.1.0 (Planning Phase)  
**Last Updated:** 2025-12-17
