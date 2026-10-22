// Command tinygocheck links every netkit package so `nx run netkit:build:tinygo`
// proves they compile for the Pico target with TinyGo.
package main

import (
	"net/netip"
	"time"

	"github.com/thiemok/tiny-dash/netkit/pkg/apiurl"
	"github.com/thiemok/tiny-dash/netkit/pkg/backoff"
	"github.com/thiemok/tiny-dash/netkit/pkg/bufshare"
	"github.com/thiemok/tiny-dash/netkit/pkg/dnsquery"
	"github.com/thiemok/tiny-dash/netkit/pkg/resolve"
)

func main() {
	u, err := apiurl.Parse("http://192.168.1.10:8080")
	if err != nil {
		panic(err)
	}
	println(u.String())
	b := backoff.Exponential{Min: time.Second, Max: time.Minute}
	println(b.Next(1).String())
	r := resolve.New(u.Host, func(string) (netip.Addr, time.Duration, error) {
		return u.Addr, 0, nil
	}, resolve.Policy{MaxTTL: time.Hour, MaxDialFailures: 2})
	addr, _, _ := r.Resolve(time.Now())
	println(addr.String())
	q, err := dnsquery.NewA("api.example.net", 2, 512)
	if err != nil {
		panic(err)
	}
	req, _ := q.AppendRequest(nil, 1)
	_, _, err = q.ParseResponse(req, 1)
	println(len(req), err.Error())
	sb := bufshare.New(make([]byte, 8))
	println(sb.Acquire(&bufshare.User{Name: "a", Idle: func() bool { return true }}) == nil)
}
