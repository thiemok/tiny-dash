// Package bufshare lends one buffer to several users that never need it at
// the same time, such as two network conns that run one after the other.
//
// A user takes the buffer over only when the previous holder reports itself
// idle, so a holder that still uses the buffer, e.g. a conn the stack can
// still write received data into, makes Acquire fail instead of letting both
// write into it.
package bufshare

import "errors"

var ErrInUse = errors.New("bufshare: buffer still in use by its previous holder")

// User is one of the buffer's users. Idle reports whether the user no longer
// touches the buffer; it is only asked while the user holds it.
type User struct {
	Name string
	Idle func() bool
}

// Buffer is a buffer shared by Users. The zero value is not usable; create
// one with New. It is not safe for concurrent use.
type Buffer struct {
	buf    []byte
	holder *User
}

func New(buf []byte) *Buffer { return &Buffer{buf: buf} }

// Bytes returns the shared buffer. Only its current holder may use it.
func (b *Buffer) Bytes() []byte { return b.buf }

// Holder returns the user that last acquired the buffer, or nil.
func (b *Buffer) Holder() *User { return b.holder }

// Acquire makes u the holder. It returns ErrInUse, keeping the current
// holder, if another user holds the buffer and is not idle.
func (b *Buffer) Acquire(u *User) error {
	if h := b.holder; h != nil && h != u && !h.Idle() {
		return ErrInUse
	}
	b.holder = u
	return nil
}
