// Command wpapsk prints the WPA2 pre-shared key for $WIFI_SSID and
// $WIFI_PASSWORD in hex, or nothing when the password is not a WPA
// passphrase (8-63 characters). tools/configure.sh compiles the key into the
// firmware, so the CYW43439 need not derive it at every join: PBKDF2 with
// 4096 iterations takes it ~2.2 s with the radio on, the RP2350 ~0.5 s.
package main

import (
	"crypto/pbkdf2"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"log"
	"os"
)

func main() {
	pass, ssid := os.Getenv("WIFI_PASSWORD"), os.Getenv("WIFI_SSID")
	if len(pass) < 8 || len(pass) > 63 {
		return
	}
	// IEEE 802.11i, H.4: PSK = PBKDF2(HMAC-SHA1, passphrase, ssid, 4096, 256 bits).
	key, err := pbkdf2.Key(sha1.New, pass, []byte(ssid), 4096, 32)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(hex.EncodeToString(key))
}
