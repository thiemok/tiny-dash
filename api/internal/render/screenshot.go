package render

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/thiemok/tiny-dash/api/internal/telemetry"
)

var tracer = otel.Tracer("github.com/thiemok/tiny-dash/api/internal/render")

// Renderer captures screenshots of URLs using a long-lived Chrome instance.
type Renderer struct {
	allocCtx    context.Context
	allocCancel context.CancelFunc
}

// NewRenderer creates a Renderer with a headless Chrome browser.
// If execPath is non-empty, it is used as the Chrome binary (e.g. a flatpak wrapper script).
func NewRenderer(execPath string) (*Renderer, error) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
	)
	if execPath != "" {
		opts = append(opts, chromedp.ExecPath(execPath))
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	// Create a persistent browser context to reuse across requests.
	browserCtx, _ := chromedp.NewContext(allocCtx)
	// Navigate to a blank page to start the browser process.
	if err := chromedp.Run(browserCtx); err != nil {
		allocCancel()
		return nil, fmt.Errorf("starting chrome: %w", err)
	}

	return &Renderer{
		allocCtx:    browserCtx,
		allocCancel: allocCancel,
	}, nil
}

// Close shuts down the Chrome browser.
func (r *Renderer) Close() {
	r.allocCancel()
}

// CaptureURL navigates to the given URL, waits for the page to settle,
// and returns a screenshot as an image.Image. The trace context from ctx is
// sent with every request Chrome makes, so the page load joins the caller's trace.
func (r *Renderer) CaptureURL(ctx context.Context, url string, width, height int, settleTime time.Duration) (image.Image, error) {
	ctx, span := tracer.Start(ctx, "render.CaptureURL", trace.WithAttributes(
		attribute.Int("tinydash.render.width", width),
		attribute.Int("tinydash.render.height", height),
	))
	defer span.End()

	tabCtx, tabCancel := chromedp.NewContext(r.allocCtx)
	defer tabCancel()

	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	headers := make(network.Headers, len(carrier))
	for k, v := range carrier {
		headers[k] = v
	}

	var screenshotBuf []byte
	err := chromedp.Run(tabCtx,
		network.Enable(),
		network.SetExtraHTTPHeaders(headers),
		chromedp.EmulateViewport(int64(width), int64(height)),
		chromedp.Navigate(url),
		chromedp.Sleep(settleTime),
		chromedp.FullScreenshot(&screenshotBuf, 100),
	)
	if err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("capturing screenshot: %w", err))
	}

	img, err := png.Decode(bytes.NewReader(screenshotBuf))
	if err != nil {
		return nil, telemetry.RecordError(span, fmt.Errorf("decoding screenshot: %w", err))
	}

	return img, nil
}
