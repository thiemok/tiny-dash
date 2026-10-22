// Package logfmt is a [slog.Handler] writing slog.TextHandler-style
// "key=value" lines without heap allocations, for TinyGo firmware loops that
// must not allocate.
//
// Records logged through [slog.Logger.LogAttrs] are formatted into a buffer
// owned by the handler and written with a single Write call. String, Int64,
// Uint64, Float64, Bool and Duration values, and error values passed with
// [slog.Any], do not allocate once the buffer has grown to the longest line.
// Other kinds fall back to [slog.Value.String], which may allocate. Note that
// the variadic ...any methods ([slog.Logger.Info] etc.) box their arguments
// and allocate before the handler is reached, and that slog.Record holds only
// five attrs inline: a LogAttrs call with more allocates inside log/slog.
package logfmt

import (
	"context"
	"io"
	"log/slog"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

const timeFormat = "2006-01-02T15:04:05.000Z07:00"

// Options configures a Handler.
type Options struct {
	// Level is the minimum level logged; nil means slog.LevelInfo.
	Level slog.Leveler
	// OmitTime drops the leading time=... field.
	OmitTime bool
	// Time, if set, replaces the record's time in the time=... field, e.g.
	// with a wall clock on a device that boots at the Unix epoch. A zero
	// result keeps the record's time. It is called with the handler's lock
	// held, so it must not log, and it must not allocate to keep Handle
	// allocation-free.
	Time func() time.Time
	// BufferSize is the initial line buffer capacity; 0 means 256.
	BufferSize int
}

type sink struct {
	mu  sync.Mutex
	w   io.Writer
	buf []byte
	// attrFn is r.Attrs' callback, bound once so Handle does not allocate a closure.
	attrFn func(slog.Attr) bool
	group  string
}

// Handler is an allocation-free logfmt [slog.Handler].
type Handler struct {
	s        *sink
	level    slog.Leveler
	omitTime bool
	time     func() time.Time
	group    string
	prefix   []byte
}

// NewHandler returns a Handler writing lines to w.
func NewHandler(w io.Writer, opts *Options) *Handler {
	var o Options
	if opts != nil {
		o = *opts
	}
	if o.Level == nil {
		o.Level = slog.LevelInfo
	}
	if o.BufferSize <= 0 {
		o.BufferSize = 256
	}
	s := &sink{w: w, buf: make([]byte, 0, o.BufferSize)}
	s.attrFn = s.appendAttr
	return &Handler{s: s, level: o.Level, omitTime: o.OmitTime, time: o.Time}
}

// WithLevel returns a Handler sharing h's writer, buffer and lock but with a
// different minimum level, e.g. a quieter one for a driver.
func (h *Handler) WithLevel(level slog.Leveler) *Handler {
	h2 := *h
	h2.level = level
	return &h2
}

func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level.Level()
}

func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	s := h.s
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.buf[:0]
	if t := h.recordTime(r); !h.omitTime && !t.IsZero() {
		b = append(b, "time="...)
		b = t.AppendFormat(b, timeFormat)
		b = append(b, ' ')
	}
	b = append(b, "level="...)
	b = append(b, r.Level.String()...)
	b = append(b, " msg="...)
	b = appendString(b, r.Message)
	b = append(b, h.prefix...)
	s.buf = b
	s.group = h.group
	r.Attrs(s.attrFn)
	s.buf = append(s.buf, '\n')
	_, err := s.w.Write(s.buf)
	return err
}

func (h *Handler) recordTime(r slog.Record) time.Time {
	if h.time != nil && !h.omitTime {
		if t := h.time(); !t.IsZero() {
			return t
		}
	}
	return r.Time
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	h2 := *h
	h.s.mu.Lock()
	saved, savedGroup := h.s.buf, h.s.group
	h.s.buf, h.s.group = append([]byte(nil), h.prefix...), h.group
	for _, a := range attrs {
		h.s.appendAttr(a)
	}
	h2.prefix = h.s.buf
	h.s.buf, h.s.group = saved, savedGroup
	h.s.mu.Unlock()
	return &h2
}

func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	h2 := *h
	h2.group = h.group + name + "."
	return &h2
}

func (s *sink) appendAttr(a slog.Attr) bool {
	v := a.Value
	if v.Kind() == slog.KindLogValuer {
		// Resolve defers a recover closure, which allocates under TinyGo.
		v = v.Resolve()
	}
	if a.Key == "" && v.Kind() == slog.KindAny && v.Any() == nil {
		return true
	}
	if v.Kind() == slog.KindGroup {
		saved := s.group
		if a.Key != "" {
			s.group = saved + a.Key + "."
		}
		for _, ga := range v.Group() {
			s.appendAttr(ga)
		}
		s.group = saved
		return true
	}
	b := append(s.buf, ' ')
	b = append(b, s.group...)
	b = appendString(b, a.Key)
	b = append(b, '=')
	s.buf = appendValue(b, v)
	return true
}

func appendValue(b []byte, v slog.Value) []byte {
	switch v.Kind() {
	case slog.KindString:
		return appendString(b, v.String())
	case slog.KindInt64:
		return strconv.AppendInt(b, v.Int64(), 10)
	case slog.KindUint64:
		return strconv.AppendUint(b, v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.AppendFloat(b, v.Float64(), 'g', -1, 64)
	case slog.KindBool:
		return strconv.AppendBool(b, v.Bool())
	case slog.KindDuration:
		return appendDuration(b, v.Duration())
	case slog.KindTime:
		return v.Time().AppendFormat(b, timeFormat)
	case slog.KindAny:
		if err, ok := v.Any().(error); ok {
			return appendString(b, err.Error())
		}
	}
	return appendString(b, v.String())
}

// appendDuration writes d in milliseconds ("1500ms") instead of
// time.Duration.String, which allocates.
func appendDuration(b []byte, d time.Duration) []byte {
	b = strconv.AppendInt(b, d.Milliseconds(), 10)
	return append(b, "ms"...)
}

func appendString(b []byte, s string) []byte {
	if needsQuoting(s) {
		return strconv.AppendQuote(b, s)
	}
	return append(b, s...)
}

func needsQuoting(s string) bool {
	if s == "" {
		return true
	}
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c <= ' ' || c == '=' || c == '"' || c == 0x7f {
				return true
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError {
			return true
		}
		i += size
	}
	return false
}
