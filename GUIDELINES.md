# tiny-dash Implementation Guidelines

This document outlines the key implementation guidelines for the tiny-dash project.

## Project Overview

tiny-dash is an e-ink dashboard system consisting of:
- **Display**: 7.3" Inky Impression Spectra 6 (800x480px, 6-color e-paper)
- **Embedded Device**: Network-enabled device running TinyGo
- **Renderer Service**: Go-based HTTP service for dashboard rendering
- **Dashboards**: Server-side rendered HTML content (weather, calendar, transport)

## Core Guidelines

### 1. Server-Side Rendering Only

**Requirement**: Dashboards must render completely on first paint with no client-side JavaScript execution.

**Rationale**: The e-ink display shows a static image snapshot. Any dynamic content must be resolved server-side.

**Implementation**:
- All data fetching happens server-side before rendering
- CSS must be inline or embedded (no external requests during render)
- No client-side JavaScript in dashboard HTML
- Use Go templates or similar for server-side HTML generation

### 2. Color Palette Constraints

**Requirement**: All content must be designed for the 6-color Spectra palette.

**Colors Available**:
- Black
- White
- Red
- Yellow
- Blue
- Green

**Implementation**:
- Renderer must map RGB colors to closest Spectra color
- Consider dithering for gradients or photographs
- Design dashboards with limited palette in mind
- Test in grayscale first to ensure readability
- Avoid subtle color differences (will be lost in conversion)

### 3. Network Resilience

**Requirement**: System must gracefully handle network failures at all levels.

**Implementation**:
- Renderer caches last successful render of each dashboard
- Device falls back to cached image if network/renderer unavailable
- Dashboards handle API failures with cached or default data
- Implement exponential backoff for retries
- Clear error states in dashboards when data unavailable

### 4. E-ink Optimization

**Requirement**: Minimize display refreshes and optimize for e-ink characteristics.

**Implementation**:
- Display refreshes only on button press or scheduled intervals
- No continuous/animated updates
- Design for high contrast (e-ink has limited grayscale range)
- Use large, readable fonts
- Consider e-ink ghosting (previous image may faintly persist)
- Explore partial refresh capabilities of Spectra 6 if applicable

### 5. Go/TinyGo Constraints

**Requirement**: Use Go/TinyGo for all components; respect TinyGo limitations.

**TinyGo Limitations**:
- Limited standard library support
- No reflection-heavy packages
- Smaller feature set than full Go
- Memory constraints on embedded devices

**Implementation**:
- Minimize heap allocations in TinyGo code
- Use standard library where possible
- Minimize external dependencies
- Test TinyGo builds early and often
- Keep embedded binary size small
- Profile memory usage on target hardware

### 6. Build System Consistency

**Requirement**: Unified build system for all packages using Make.

**Implementation**:
- Single `make all` builds all packages
- Separate targets for Go (native) and TinyGo (embedded) builds
- Consistent version management across packages
- Reproducible builds (pin dependency versions)
- Clear separation of build artifacts
- Easy development workflow (`make dev`, `make test`, etc.)

### 7. Image Format Specification

**Requirement**: Standardized image format for renderer ↔ device communication.

**Specification**:
- **Format**: Uncompressed bitmap (BMP)
- **Resolution**: 800x480 pixels
- **Color Depth**: 6-color indexed palette
- **Palette Order**: Black, White, Red, Yellow, Blue, Green
- **Transfer**: HTTP GET endpoint with proper content-type headers
- **Naming**: Consider version/hash in image URLs for cache control

## Package-Specific Guidelines

### Renderer (cmd/renderer)
- Use headless Chrome/Chromium for HTML rendering
- Implement efficient RGB-to-Spectra color conversion
- Expose RESTful API with clear versioning
- Log all dashboard render attempts and failures

### Device (cmd/device)
- Implement robust HTTP client with timeouts
- Handle button debouncing in GPIO code
- Minimize memory allocations
- Graceful shutdown on errors

### E-ink Driver (pkg/eink)
- Port Pimoroni Python library accurately
- Abstract hardware layer for testing
- Document SPI protocol and timing requirements
- Provide clear error messages for hardware issues

### Dashboards (pkg/dashboards)
- Each dashboard is independently testable
- Clear data source abstraction for mocking
- Consistent HTML structure across dashboards
- Include metadata (last update time, data freshness)

### Tools (tools/)
- Automate repetitive tasks
- Provide helpful error messages
- Document all make targets in README
- Include deployment automation for embedded device

## Testing Strategy

- **Unit Tests**: All Go packages (standard `go test`)
- **Integration Tests**: Renderer + dashboard combinations
- **Hardware Tests**: E-ink driver on actual hardware
- **Visual Tests**: Screenshot comparisons for dashboard rendering
- **TinyGo Build Tests**: Verify embedded code compiles regularly

## Development Workflow

1. Develop dashboards with local renderer
2. Test rendering output visually
3. Test color conversion accuracy
4. Deploy renderer to network-accessible server
5. Flash device firmware with TinyGo
6. Test full integration on hardware

## Multi-Module Architecture Guidelines

### Module Organization

The project uses a multi-module monorepo structure with separate Go modules:

**Standard Go modules** (renderer, dashboards):
- Use standard Go toolchain
- Built with Nx using `@nx-go/nx-go` plugin
- Can use any Go library

**TinyGo modules** (device, eink):
- Compatible with TinyGo constraints
- Built with Nx using shell scripts
- Limited to TinyGo-compatible libraries

### Module Dependencies

**Dependency Management:**
- Use `replace` directives in `go.mod` for local development
- Each module has its own `go.mod` and version
- Dependencies flow: `renderer` → `dashboards`, `device` → `eink`
- No circular dependencies allowed

**Example go.mod with replace:**
```go
module github.com/user/tiny-dash/renderer

require (
    github.com/user/tiny-dash/dashboards v0.0.0
)

replace github.com/user/tiny-dash/dashboards => ../dashboards
```

### Nx Build System Guidelines

**Build orchestration:**
- Use Nx for all build operations (`nx build`, `nx test`, etc.)
- Standard Go modules use `@nx-go/nx-go` executor
- TinyGo modules use `nx:run-commands` with shell scripts
- Leverage Nx caching for faster rebuilds

**Common commands:**
```bash
nx run-many --target=build --all    # Build all modules
nx build renderer                   # Build specific module
nx affected:build                   # Build only affected modules
nx graph                            # View dependency graph
```

**Configuration:**
- Each module has `project.json` for Nx configuration
- Root `nx.json` defines workspace-wide settings
- Build scripts in `tools/` for TinyGo modules

### Module Independence

**Standalone modules:**
- `eink` and `dashboards` can be published separately
- No dependencies on other tiny-dash modules
- Can be used in other projects

**Dependent modules:**
- `renderer` depends on `dashboards`
- `device` depends on `eink`
- Keep dependencies minimal

## Future Considerations

- Dashboard rotation scheduling (time-based, not just button)
- Partial display refresh for faster updates
- Battery-powered operation (deep sleep modes)
- Dashboard configuration via web UI
- Multiple display support
- Publishing standalone modules to Go package registry
- CI/CD integration with Nx
- Multi-architecture builds (AMD64, ARM64)

---

**Last Updated**: 2025-12-17
**Version**: 1.1 (Multi-module architecture)
