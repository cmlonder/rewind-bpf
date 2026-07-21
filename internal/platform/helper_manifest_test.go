package platform

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestVerifyNativeHelperRequiresExactBytesAndPlatform(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("native helper verification is target-platform specific")
	}
	root := t.TempDir()
	helper := filepath.Join(root, "rewind-native-helper")
	content := []byte("signed helper fixture\n")
	if err := os.WriteFile(helper, content, 0o755); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(content)
	manifestPath := filepath.Join(root, "helper.json")
	manifest := HelperManifest{Version: NativeHelperManifestVersion, Platform: runtime.GOOS, Name: "fixture", HelperPath: helper, HelperSHA256: hex.EncodeToString(digest[:])}
	data, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	verified, err := VerifyNativeHelper(manifestPath, nil)
	if err != nil || !verified.Verified {
		t.Fatalf("verification=%+v err=%v", verified, err)
	}
	if err := os.WriteFile(helper, []byte("tampered\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tampered, err := VerifyNativeHelper(manifestPath, nil)
	if err != nil || tampered.Verified || tampered.ChecksumMatch {
		t.Fatalf("tampered helper was accepted: %+v err=%v", tampered, err)
	}
}

func TestVerifyNativeHelperReportsForeignPlatformWithoutReadingHelper(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "helper.json")
	data, _ := json.Marshal(HelperManifest{Version: NativeHelperManifestVersion, Platform: "windows", Name: "fixture", HelperPath: filepath.Join(root, "missing.exe"), HelperSHA256: "deadbeef"})
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	verification, err := VerifyNativeHelper(manifestPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if verification.PlatformMatch == false {
			t.Fatal("Windows host unexpectedly reported foreign platform")
		}
	} else if verification.PlatformMatch || verification.Verified {
		t.Fatalf("foreign helper unexpectedly accepted: %+v", verification)
	}
}
