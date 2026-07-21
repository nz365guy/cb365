//go:build linux && cgo && managed_evidence

package integration

import (
	"os"
	"testing"

	"github.com/nz365guy/cb365/internal/auth"
)

func TestManagedEvidenceEnvironmentIsCredentialFree(t *testing.T) {
	for _, key := range []string{
		"BWS_ACCESS_TOKEN",
		"AZURE_CLIENT_SECRET",
		"AZURE_CLIENT_CERTIFICATE_PASSWORD",
		"CB365_KEYRING_PASSWORD",
	} {
		if os.Getenv(key) != "" {
			t.Fatalf("credential-bearing environment key %s must be absent", key)
		}
	}
}

func TestManagedDelegatedT3ToT6Evidence(t *testing.T) {
	results, err := auth.RunManagedEvidenceSuite(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"t3a", "t3b"} {
		result := results[name]
		if result.Class != auth.ManagedCacheUnavailable || result.EntraRequests != 0 ||
			result.WorkloadRequests != 0 || result.PromptCalls != 0 || result.LegacyReads != 0 {
			t.Fatalf("%s did not fail closed before every downstream request: %+v", name, result)
		}
	}
	if results["t4"].SentinelResidue {
		t.Fatal("T4 found sentinel or token-shaped residue outside the cleared fake record")
	}
	if result := results["t5a"]; result.Class != auth.ManagedCacheConflict || result.FinalGeneration != 2 || result.StoreUpdates != 1 {
		t.Fatalf("T5a did not preserve one-writer generation invariants: %+v", result)
	}
	if result := results["t5b"]; result.Class != auth.ManagedCacheConflict || result.FinalGeneration != 3 || result.StoreUpdates != 0 {
		t.Fatalf("T5b stale writer changed the winning record: %+v", result)
	}
	if result := results["t6a"]; result.LegacyReads != 1 || result.SourceRetained {
		t.Fatalf("T6a did not verify, migrate, read back, then delete the legacy source: %+v", result)
	}
	if result := results["t6b-permission"]; result.Class != auth.ManagedCacheInvalid || result.LegacyReads != 0 || !result.SourceRetained {
		t.Fatalf("T6b permission failure did not stop before legacy read: %+v", result)
	}
	if result := results["t6b-readback"]; result.Class != auth.ManagedCacheConflict || !result.SourceRetained {
		t.Fatalf("T6b readback failure did not preserve the legacy source: %+v", result)
	}
	if result := results["t6c"]; result.SourceRetained || result.SentinelResidue {
		t.Fatalf("T6c logout evidence retained delegated material: %+v", result)
	}
}
