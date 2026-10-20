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

func TestHandlerTimeHook(t *testing.T) {
	var out bytes.Buffer
	var wall time.Time
	h := NewHandler(&out, &Options{Time: func() time.Time { return wall }})
	l := slog.New(h.WithLevel(slog.LevelInfo))
	recordTime := time.Unix(5, 0).UTC()
	r := slog.NewRecord(recordTime, slog.LevelInfo, "boot", 0)
	if err := l.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	wall = time.Date(2026, 10, 20, 9, 30, 1, 250_000_000, time.UTC)
	r = slog.NewRecord(recordTime, slog.LevelInfo, "synced", 0)
	if err := l.Handler().Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	want := "time=1970-01-01T00:00:05.000Z level=INFO msg=boot\n" +
		"time=2026-10-20T09:30:01.250Z level=INFO msg=synced\n"
	if got := out.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	out.Reset()
	NewHandler(&out, &Options{OmitTime: true, Time: func() time.Time { return wall }}).Handle(context.Background(), r)
	if got := out.String(); got != "level=INFO msg=synced\n" {
		t.Errorf("OmitTime with a Time hook: got %q", got)
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

func TestHandlerTimeHookDoesNotAllocate(t *testing.T) {
	wall := time.Date(2026, 10, 20, 9, 30, 1, 0, time.UTC)
	l := slog.New(NewHandler(discard{}, &Options{Time: func() time.Time { return wall }}))
	ctx := context.Background()
	allocs := testing.AllocsPerRun(100, func() {
		l.LogAttrs(ctx, slog.LevelInfo, "cycle", slog.Int("n", 12))
		wall = wall.Add(time.Second)
	})
	if allocs != 0 {
		t.Errorf("LogAttrs with a Time hook allocated %v times per run, want 0", allocs)
	}
}
