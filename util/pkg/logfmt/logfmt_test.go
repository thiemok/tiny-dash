package logfmt

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
)

func TestHandlerFormat(t *testing.T) {
	var out bytes.Buffer
	l := slog.New(NewHandler(&out, &Options{OmitTime: true, Level: slog.LevelDebug}))
	l.LogAttrs(context.Background(), slog.LevelInfo, "display updated",
		slog.String("host", "dash.example.net:8080"),
		slog.String("quoted", `a "b"=c`),
		slog.String("empty", ""),
		slog.Int("n", -3),
		slog.Uint64("mallocs", 1<<40),
		slog.Bool("ok", true),
		slog.Float64("f", 1.5),
		slog.Duration("took", 1500*time.Millisecond),
		slog.Any("error", errors.New("tcp dial: timeout")),
		slog.Group("heap", slog.Uint64("alloc", 7)),
	)
	l.With("conn", 1).WithGroup("g").Debug("✓ ready", "k", "v")

	want := `level=INFO msg="display updated" host=dash.example.net:8080 quoted="a \"b\"=c" empty="" n=-3 mallocs=1099511627776 ok=true f=1.5 took=1500ms error="tcp dial: timeout" heap.alloc=7` + "\n" +
		`level=DEBUG msg="✓ ready" conn=1 g.k=v` + "\n"
	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestHandlerLevel(t *testing.T) {
	var out bytes.Buffer
	h := NewHandler(&out, &Options{OmitTime: true})
	slog.New(h).Debug("hidden")
	slog.New(h.WithLevel(slog.LevelWarn)).Info("hidden")
	slog.New(h.WithLevel(slog.LevelWarn)).Warn("shown")
	if got, want := out.String(), "level=WARN msg=shown\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHandlerTime(t *testing.T) {
	var out bytes.Buffer
	h := NewHandler(&out, nil)
	r := slog.NewRecord(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC), slog.LevelInfo, "m", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got, want := out.String(), "time=2026-10-18T12:00:00.000Z level=INFO msg=m\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestHandlerLogAttrsDoesNotAllocate(t *testing.T) {
	l := slog.New(NewHandler(discard{}, nil))
	ctx := context.Background()
	err := errors.New("fetch failed")
	allocs := testing.AllocsPerRun(100, func() {
		l.LogAttrs(ctx, slog.LevelInfo, "cycle",
			slog.Int("cycle", 12),
			slog.Uint64("mallocs", 1<<40),
			slog.Bool("ok", true),
			slog.Duration("took", time.Second),
			slog.Any("error", err),
		)
		l.LogAttrs(ctx, slog.LevelDebug, "disabled", slog.Int("n", 1))
	})
	if allocs != 0 {
		t.Errorf("LogAttrs allocated %v times per run, want 0", allocs)
	}
}
