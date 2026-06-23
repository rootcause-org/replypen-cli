package cli

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"v0.1.0", "0.1.0", 0}, // leading v ignored
		{"0.1.0", "0.2.0", -1},
		{"0.2.0", "0.1.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.1.0-rc1", "0.1.0", 0}, // patch suffix tolerated; numerics equal
		{"dev", "0.1.0", -1},      // unparseable → assume an update is available
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestAssetName(t *testing.T) {
	if got := assetName("0.3.1", "darwin", "arm64"); got != "rp_0.3.1_darwin_arm64.tar.gz" {
		t.Errorf("darwin asset = %q", got)
	}
	if got := assetName("0.3.1", "windows", "amd64"); got != "rp_0.3.1_windows_amd64.zip" {
		t.Errorf("windows asset = %q", got)
	}
}

func TestBinaryName(t *testing.T) {
	if binaryName("linux") != "rp" || binaryName("windows") != "rp.exe" {
		t.Errorf("binaryName wrong: %q / %q", binaryName("linux"), binaryName("windows"))
	}
}

func TestSha256FromChecksums(t *testing.T) {
	data := []byte("abc123  rp_0.3.1_darwin_arm64.tar.gz\ndef456  rp_0.3.1_linux_amd64.tar.gz\n")
	got, err := sha256FromChecksums(data, "rp_0.3.1_linux_amd64.tar.gz")
	if err != nil || got != "def456" {
		t.Errorf("got %q err %v", got, err)
	}
	if _, err := sha256FromChecksums(data, "rp_9.9.9_plan9_386.tar.gz"); err == nil {
		t.Error("expected error for a missing asset")
	}
}

func TestIsHomebrewManaged(t *testing.T) {
	if !isHomebrewManaged("/opt/homebrew/Cellar/rp/0.3.1/bin/rp") {
		t.Error("Cellar path should be brew-managed")
	}
	if !isHomebrewManaged("/usr/local/Caskroom/rp/0.3.1/rp") {
		t.Error("Caskroom path should be brew-managed")
	}
	if isHomebrewManaged("/usr/local/bin/rp") {
		t.Error("a plain bin path is not brew-managed")
	}
}
