// genkey prints a random base64-encoded 32-byte key for CREDENTIAL_ENCRYPTION_KEY.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
)

func main() {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(base64.StdEncoding.EncodeToString(key))
}
