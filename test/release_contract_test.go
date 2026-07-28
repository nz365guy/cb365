package releasecontract_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCosignV3ReleaseUsesBundleContract(t *testing.T) {
	repositoryRoot := filepath.Join("..")
	goreleaserConfig := readRepositoryFile(t, repositoryRoot, ".goreleaser.yaml")
	releaseWorkflow := readRepositoryFile(t, repositoryRoot, ".github", "workflows", "release.yml")

	requiredConfig := []string{
		`signature: "${artifact}.sigstore.json"`,
		`"--bundle=${signature}"`,
		`--bundle checksums.txt.sigstore.json`,
	}
	for _, expected := range requiredConfig {
		if !strings.Contains(goreleaserConfig, expected) {
			t.Errorf(".goreleaser.yaml must contain %q", expected)
		}
	}

	deprecatedConfig := []string{
		`certificate: "${artifact}.pem"`,
		"--output-certificate",
		"--output-signature",
		"--certificate checksums.txt.pem",
		"--signature checksums.txt.sig",
	}
	for _, deprecated := range deprecatedConfig {
		if strings.Contains(goreleaserConfig, deprecated) {
			t.Errorf(".goreleaser.yaml still contains Cosign v2 setting %q", deprecated)
		}
	}

	const cosignInstallerV4 = "sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2"
	if !strings.Contains(releaseWorkflow, cosignInstallerV4) {
		t.Errorf("release workflow must install the reviewed Cosign v3 runtime through %q", cosignInstallerV4)
	}
}

func readRepositoryFile(t *testing.T, pathParts ...string) string {
	t.Helper()
	path := filepath.Join(pathParts...)
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(contents)
}
