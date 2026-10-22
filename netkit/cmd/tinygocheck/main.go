// Command tinygocheck links every netkit package so `nx run netkit:build:tinygo`
// proves they compile for the Pico target with TinyGo.
package main

import "github.com/thiemok/tiny-dash/netkit/pkg/apiurl"

func main() {
	u, err := apiurl.Parse("http://192.168.1.10:8080")
	if err != nil {
		panic(err)
	}
	println(u.String())
}
