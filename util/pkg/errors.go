package util

import "fmt"

// RequireNoError panics if err is not nil, optionally including a custom message
func RequireNoError(err error, msg ...string) {
	if err != nil {
		if len(msg) > 0 {
			panic(fmt.Sprintf("%s: %v", msg[0], err))
		}
		panic(err)
	}
}
