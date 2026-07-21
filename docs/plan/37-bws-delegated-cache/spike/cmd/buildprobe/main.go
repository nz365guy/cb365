// buildprobe is the Gate 2 evidence binary for cb365 #42. Built twice:
//
//	CGO_ENABLED=1 — must report the provider available and construct (then
//	                close) a real Bitwarden SDK client, proving static
//	                linking of libbitwarden_c on the target toolchain.
//	CGO_ENABLED=0 — must report the provider unavailable with the exact
//	                ADR-0057 stub error, proving the fail-closed path.
//
// It performs no network call and touches no secret.
package main

import (
	"fmt"
	"os"

	"github.com/nz365guy/cb365-spike37/bwsadapter"
)

func main() {
	fmt.Printf("cgo_provider_available: %v\n", bwsadapter.Available())

	adapter, err := bwsadapter.New("https://api.bitwarden.eu", "https://identity.bitwarden.eu")
	if err != nil {
		fmt.Printf("provider_construct: refused (%v)\n", err)
		if bwsadapter.Available() {
			// A cgo build that cannot construct the client is a Gate 2 FAIL.
			os.Exit(1)
		}
		return
	}
	adapter.Close()
	fmt.Println("provider_construct: ok (SDK client constructed and closed; no network, no secret access)")
}
