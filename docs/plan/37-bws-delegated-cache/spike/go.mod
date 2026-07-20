// Spike module for cb365 #42 (ADR-0057 pre-implementation gates).
// Deliberately separate from the cb365 root module: ADR-0057 authorises
// this spike only; the Bitwarden SDK must not enter the cb365 build until
// the G1 implementation item is approved.
module github.com/nz365guy/cb365-spike37

go 1.25.0

require (
	github.com/AzureAD/microsoft-authentication-library-for-go v1.7.2
	github.com/bitwarden/sdk-go/v2 v2.1.0
)
