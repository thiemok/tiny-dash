package bufshare

import "testing"

type fakeConn struct{ open bool }

func (c *fakeConn) user(name string) *User {
	return &User{Name: name, Idle: func() bool { return !c.open }}
}

func TestAcquire(t *testing.T) {
	var tcp, udp fakeConn
	tcpUser, udpUser := tcp.user("tcp"), udp.user("udp")
	b := New(make([]byte, 16))

	if err := b.Acquire(tcpUser); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	tcp.open = true
	if err := b.Acquire(tcpUser); err != nil {
		t.Errorf("holder re-acquiring: %v", err)
	}
	if err := b.Acquire(udpUser); err != ErrInUse {
		t.Errorf("Acquire while the holder is open = %v, want ErrInUse", err)
	}
	if b.Holder() != tcpUser {
		t.Errorf("failed Acquire changed the holder to %s", b.Holder().Name)
	}

	tcp.open = false
	if err := b.Acquire(udpUser); err != nil {
		t.Fatalf("Acquire after the holder closed: %v", err)
	}
	udp.open = true
	if err := b.Acquire(tcpUser); err != ErrInUse {
		t.Errorf("Acquire while the new holder is open = %v, want ErrInUse", err)
	}
	udp.open = false
	if err := b.Acquire(tcpUser); err != nil || b.Holder() != tcpUser {
		t.Errorf("handing back: err %v, holder %s", err, b.Holder().Name)
	}
}

func TestAcquireDoesNotAllocate(t *testing.T) {
	var tcp, udp fakeConn
	tcpUser, udpUser := tcp.user("tcp"), udp.user("udp")
	b := New(make([]byte, 16))
	allocs := testing.AllocsPerRun(100, func() {
		b.Acquire(tcpUser)
		tcp.open = true
		b.Acquire(udpUser)
		tcp.open = false
		b.Acquire(udpUser)
	})
	if allocs != 0 {
		t.Errorf("Acquire allocated %v times per run", allocs)
	}
}
