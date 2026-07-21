package platform

// This file contains the portable, read-only trust gate for the native
// helper.  The helper itself is intentionally an external signed artifact:
// EndpointSecurity/APFS and Windows minifilter/VHDX APIs cannot be emulated by
// the Go process without weakening the safety boundary.  Rewind therefore
// verifies the exact helper bytes before a platform adapter is allowed to
// consider it.

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rewindbpf/rewind/internal/release"
)

const NativeHelperManifestVersion = 1

// HelperManifest is distributed beside a platform helper binary.  The
// checksum is mandatory even when a detached signature is used so operators
// can inspect and reproduce the exact bytes with ordinary SHA-256 tooling.
type HelperManifest struct {
	Version       int    `json:"version"`
	Platform      string `json:"platform"`
	Name          string `json:"name"`
	HelperPath    string `json:"helper_path"`
	HelperSHA256  string `json:"helper_sha256"`
	SignaturePath string `json:"signature_path,omitempty"`
}

type HelperVerification struct {
	Manifest       HelperManifest `json:"manifest"`
	PlatformMatch  bool           `json:"platform_match"`
	ChecksumMatch  bool           `json:"checksum_match"`
	SignatureValid bool           `json:"signature_valid"`
	Verified       bool           `json:"verified"`
	Reasons        []string       `json:"reasons,omitempty"`
}

// VerifyNativeHelper reads only metadata and helper bytes.  It never starts a
// process, changes ACLs, loads a kernel extension, or installs a service.
// trusted may be empty for integrity-only verification; a non-empty set pins
// the expected publisher key using the existing release signature envelope.
func VerifyNativeHelper(manifestPath string, trusted []ed25519.PublicKey) (HelperVerification, error) {
	manifestAbs, err := filepath.Abs(strings.TrimSpace(manifestPath))
	if err != nil {
		return HelperVerification{}, fmt.Errorf("resolve native helper manifest: %w", err)
	}
	data, err := os.ReadFile(manifestAbs)
	if err != nil {
		return HelperVerification{}, fmt.Errorf("read native helper manifest: %w", err)
	}
	var manifest HelperManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return HelperVerification{}, fmt.Errorf("decode native helper manifest: %w", err)
	}
	if manifest.Version != NativeHelperManifestVersion {
		return HelperVerification{}, fmt.Errorf("unsupported native helper manifest version %d", manifest.Version)
	}
	manifest.Platform = strings.ToLower(strings.TrimSpace(manifest.Platform))
	manifest.Name = strings.TrimSpace(manifest.Name)
	if manifest.Platform != "darwin" && manifest.Platform != "windows" {
		return HelperVerification{}, fmt.Errorf("native helper platform must be darwin or windows")
	}
	if manifest.Name == "" {
		return HelperVerification{}, fmt.Errorf("native helper name is required")
	}
	if strings.TrimSpace(manifest.HelperPath) == "" {
		return HelperVerification{}, fmt.Errorf("native helper path is required")
	}
	helperAbs, err := filepath.Abs(manifest.HelperPath)
	if err != nil {
		return HelperVerification{}, fmt.Errorf("resolve native helper path: %w", err)
	}
	manifest.HelperPath = filepath.Clean(helperAbs)
	verification := HelperVerification{Manifest: manifest, PlatformMatch: manifest.Platform == runtime.GOOS}
	if !verification.PlatformMatch {
		verification.Reasons = append(verification.Reasons, fmt.Sprintf("helper targets %s but host is %s", manifest.Platform, runtime.GOOS))
		return verification, nil
	}
	info, err := os.Stat(manifest.HelperPath)
	if err != nil {
		verification.Reasons = append(verification.Reasons, fmt.Sprintf("helper is unavailable: %v", err))
		return verification, nil
	}
	if info.IsDir() {
		verification.Reasons = append(verification.Reasons, "helper path is a directory")
		return verification, nil
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		verification.Reasons = append(verification.Reasons, "helper is not executable")
		return verification, nil
	}
	helperBytes, err := os.ReadFile(manifest.HelperPath)
	if err != nil {
		verification.Reasons = append(verification.Reasons, fmt.Sprintf("read helper: %v", err))
		return verification, nil
	}
	digest := sha256.Sum256(helperBytes)
	actual := hex.EncodeToString(digest[:])
	expected := strings.ToLower(strings.TrimSpace(manifest.HelperSHA256))
	verification.ChecksumMatch = expected != "" && expected == actual
	if !verification.ChecksumMatch {
		verification.Reasons = append(verification.Reasons, "helper SHA-256 does not match manifest")
	}
	if strings.TrimSpace(manifest.SignaturePath) == "" {
		verification.SignatureValid = len(trusted) == 0
		if len(trusted) > 0 {
			verification.Reasons = append(verification.Reasons, "trusted verification requested but signature_path is empty")
		}
	} else {
		signaturePath := manifest.SignaturePath
		if !filepath.IsAbs(signaturePath) {
			signaturePath = filepath.Join(filepath.Dir(manifestAbs), signaturePath)
		}
		signaturePath, err := filepath.Abs(signaturePath)
		if err != nil {
			verification.Reasons = append(verification.Reasons, fmt.Sprintf("resolve helper signature: %v", err))
		} else if sigData, readErr := os.ReadFile(filepath.Clean(signaturePath)); readErr != nil {
			verification.Reasons = append(verification.Reasons, fmt.Sprintf("read helper signature: %v", readErr))
		} else {
			var signature release.Signature
			if decodeErr := json.Unmarshal(sigData, &signature); decodeErr != nil {
				verification.Reasons = append(verification.Reasons, fmt.Sprintf("decode helper signature: %v", decodeErr))
			} else if verifyErr := release.Verify(helperBytes, signature, trusted...); verifyErr != nil {
				verification.Reasons = append(verification.Reasons, verifyErr.Error())
			} else {
				verification.SignatureValid = true
			}
		}
	}
	verification.Verified = verification.PlatformMatch && verification.ChecksumMatch && verification.SignatureValid
	if !verification.Verified && len(verification.Reasons) == 0 {
		verification.Reasons = append(verification.Reasons, "native helper trust gate failed")
	}
	return verification, nil
}
