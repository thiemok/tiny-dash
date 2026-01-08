package main

import (
	"time"
)

func main() {
	println("Hello from tiny-dash device!")
	println("Target hardware initialized")

	counter := 0
	for {
		println("Loop iteration:", counter)
		counter++
		time.Sleep(time.Second)
	}
}
