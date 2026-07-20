package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	evidencebundle "github.com/rewindbpf/rewind/internal/bundle"
	releasecrypto "github.com/rewindbpf/rewind/internal/release"
	"github.com/rewindbpf/rewind/internal/runstore"
)

func handleBundle(args []string) {
	if len(args) == 0 {
		fatal("usage: rewind bundle create --record PATH --output PATH | rewind bundle verify --input PATH")
	}
	if args[0] == "verify" {
		handleBundleVerify(args[1:])
		return
	}
	if args[0] == "sign" {
		handleBundleSign(args[1:])
		return
	}
	if args[0] == "encrypt" {
		handleBundleEncrypt(args[1:])
		return
	}
	if args[0] == "decrypt" {
		handleBundleDecrypt(args[1:])
		return
	}
	if args[0] == "publish" {
		handleBundlePublish(args[1:])
		return
	}
	if args[0] == "fetch" {
		handleBundleFetch(args[1:])
		return
	}
	if args[0] != "create" {
		fatal("usage: rewind bundle create --record PATH --output PATH | rewind bundle encrypt --input PATH --output PATH --key-file PATH | rewind bundle decrypt --input PATH --output PATH --key-file PATH | rewind bundle sign --input PATH --private-key PATH --output PATH | rewind bundle publish --input PATH --endpoint URL --signature PATH | rewind bundle verify --input PATH [--signature PATH --public-key PATH]")
	}
	flags := flag.NewFlagSet("bundle create", flag.ContinueOnError)
	recordPath := flags.String("record", "", "run record JSON path")
	outputPath := flags.String("output", "", "evidence .tar.gz output path")
	if err := flags.Parse(args[1:]); err != nil {
		fatal(err.Error())
	}
	if flags.NArg() != 0 || strings.TrimSpace(*recordPath) == "" || strings.TrimSpace(*outputPath) == "" {
		fatal("usage: rewind bundle create --record PATH --output PATH")
	}
	record, err := runstore.Read(*recordPath)
	if err != nil {
		fatal(err.Error())
	}
	outputAbs, err := filepath.Abs(*outputPath)
	if err != nil {
		fatal(fmt.Sprintf("resolve bundle output: %v", err))
	}
	if pathWithin(record.Plan.Layout.Root, outputAbs) {
		fatal("bundle output must be outside the run runtime root")
	}
	metadata, err := evidencebundle.Create(outputAbs, *recordPath, record.Plan.Layout.Root, record)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("wrote evidence bundle: %s artifacts=%d run_id=%s\n", outputAbs, len(metadata.Artifacts), metadata.RunID)
}

func handleBundleFetch(args []string) {
	flags := flag.NewFlagSet("bundle fetch", flag.ContinueOnError)
	endpoint := flags.String("endpoint", "", "HTTPS evidence endpoint")
	output := flags.String("output", "", "downloaded envelope/archive output path")
	bearer := flags.String("token", "", "optional bearer token")
	expectedSHA := flags.String("sha256", "", "optional expected SHA-256 digest")
	allowInsecure := flags.Bool("allow-insecure-localhost", false, "allow HTTP only for localhost test endpoints")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*endpoint) == "" || strings.TrimSpace(*output) == "" {
		fatal("usage: rewind bundle fetch --endpoint URL --output PATH [--token TOKEN --sha256 HEX --allow-insecure-localhost]")
	}
	if err := evidencebundle.Fetch(context.Background(), *endpoint, *bearer, *expectedSHA, filepath.Clean(*output), *allowInsecure); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("fetched signed evidence payload: %s\n", filepath.Clean(*output))
}

func handleBundlePublish(args []string) {
	flags := flag.NewFlagSet("bundle publish", flag.ContinueOnError)
	inputPath := flags.String("input", "", "evidence .tar.gz path")
	endpoint := flags.String("endpoint", "", "HTTPS review endpoint")
	signaturePath := flags.String("signature", "", "detached signature JSON path")
	publicPath := flags.String("public-key", "", "optional pinned public key path")
	trustedPaths := flags.String("trusted-public-keys", "", "optional comma-separated public key paths for trust rotation")
	tokenPath := flags.String("token-file", "", "optional bearer token file")
	encrypted := flags.Bool("encrypted", false, "publish an encrypted envelope instead of a plaintext evidence archive")
	allowInsecure := flags.Bool("allow-insecure-localhost", false, "allow HTTP only for localhost test endpoints")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*inputPath) == "" || strings.TrimSpace(*endpoint) == "" || strings.TrimSpace(*signaturePath) == "" {
		fatal("usage: rewind bundle publish --input PATH --endpoint URL --signature PATH [--public-key PATH --trusted-public-keys PATH,... --token-file PATH --encrypted --allow-insecure-localhost]")
	}
	if !*encrypted {
		if _, err := evidencebundle.Verify(*inputPath); err != nil {
			fatal(fmt.Sprintf("evidence bundle verification failed: %v", err))
		}
	}
	payload, err := os.ReadFile(filepath.Clean(*inputPath))
	if err != nil {
		fatal(fmt.Sprintf("read evidence bundle: %v", err))
	}
	signatureData, err := os.ReadFile(filepath.Clean(*signaturePath))
	if err != nil {
		fatal(fmt.Sprintf("read evidence bundle signature: %v", err))
	}
	var signed releasecrypto.Signature
	if err := json.Unmarshal(signatureData, &signed); err != nil {
		fatal(fmt.Sprintf("decode evidence bundle signature: %v", err))
	}
	var trustedKeys []ed25519.PublicKey
	if strings.TrimSpace(*publicPath) != "" {
		trustedKeys = append(trustedKeys, readPublicKey(*publicPath, "evidence bundle")...)
	}
	for _, path := range strings.Split(*trustedPaths, ",") {
		if strings.TrimSpace(path) != "" {
			trustedKeys = append(trustedKeys, readPublicKey(path, "evidence bundle")...)
		}
	}
	err = releasecrypto.VerifyAny(payload, signed, trustedKeys)
	if err != nil {
		fatal(fmt.Sprintf("evidence bundle signature verification failed: %v", err))
	}
	var token string
	if strings.TrimSpace(*tokenPath) != "" {
		data, err := os.ReadFile(filepath.Clean(*tokenPath))
		if err != nil {
			fatal(fmt.Sprintf("read publish token: %v", err))
		}
		token = strings.TrimSpace(string(data))
	}
	receipt, err := evidencebundle.Publish(context.Background(), *inputPath, *endpoint, token, string(signatureData), *allowInsecure)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("published signed evidence bundle: endpoint=%s key_id=%s receipt=%s\n", *endpoint, signed.KeyID, receipt)
}

func handleBundleVerify(args []string) {
	flags := flag.NewFlagSet("bundle verify", flag.ContinueOnError)
	inputPath := flags.String("input", "", "evidence .tar.gz path")
	signaturePath := flags.String("signature", "", "optional detached signature JSON path")
	publicPath := flags.String("public-key", "", "optional pinned public key path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*inputPath) == "" {
		fatal("usage: rewind bundle verify --input PATH [--signature PATH --public-key PATH]")
	}
	metadata, err := evidencebundle.Verify(*inputPath)
	if err != nil {
		fatal(fmt.Sprintf("evidence bundle verification failed: %v", err))
	}
	trust := "checksum-only"
	if strings.TrimSpace(*signaturePath) != "" {
		payload, err := os.ReadFile(filepath.Clean(*inputPath))
		if err != nil {
			fatal(fmt.Sprintf("read evidence bundle for signature: %v", err))
		}
		signatureData, err := os.ReadFile(filepath.Clean(*signaturePath))
		if err != nil {
			fatal(fmt.Sprintf("read evidence bundle signature: %v", err))
		}
		var signed releasecrypto.Signature
		if err := json.Unmarshal(signatureData, &signed); err != nil {
			fatal(fmt.Sprintf("decode evidence bundle signature: %v", err))
		}
		var trusted ed25519.PublicKey
		if strings.TrimSpace(*publicPath) != "" {
			trustedData, err := os.ReadFile(filepath.Clean(*publicPath))
			if err != nil {
				fatal(fmt.Sprintf("read evidence bundle public key: %v", err))
			}
			trusted = ed25519.PublicKey(trustedData)
		}
		if len(trusted) > 0 {
			err = releasecrypto.Verify(payload, signed, trusted)
			trust = "pinned-signature"
		} else {
			err = releasecrypto.Verify(payload, signed)
			trust = "embedded-signature"
		}
		if err != nil {
			fatal(fmt.Sprintf("evidence bundle signature verification failed: %v", err))
		}
	}
	fmt.Printf("evidence bundle verified: run_id=%s artifacts=%d state=%s trust=%s\n", metadata.RunID, len(metadata.Artifacts), metadata.State, trust)
}

func handleBundleEncrypt(args []string) {
	flags := flag.NewFlagSet("bundle encrypt", flag.ContinueOnError)
	inputPath := flags.String("input", "", "plaintext evidence archive path")
	outputPath := flags.String("output", "", "encrypted envelope output path")
	keyPath := flags.String("key-file", "", "raw 32-byte AES key path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*inputPath) == "" || strings.TrimSpace(*outputPath) == "" || strings.TrimSpace(*keyPath) == "" {
		fatal("usage: rewind bundle encrypt --input PATH --output PATH --key-file PATH")
	}
	key := readBundleKey(*keyPath)
	envelope, err := evidencebundle.EncryptFile(*inputPath, *outputPath, key)
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("encrypted evidence bundle: %s plaintext_sha256=%s\n", *outputPath, envelope.PlaintextSHA)
}

func handleBundleDecrypt(args []string) {
	flags := flag.NewFlagSet("bundle decrypt", flag.ContinueOnError)
	inputPath := flags.String("input", "", "encrypted envelope path")
	outputPath := flags.String("output", "", "plaintext evidence archive output path")
	keyPath := flags.String("key-file", "", "raw 32-byte AES key path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*inputPath) == "" || strings.TrimSpace(*outputPath) == "" || strings.TrimSpace(*keyPath) == "" {
		fatal("usage: rewind bundle decrypt --input PATH --output PATH --key-file PATH")
	}
	if err := evidencebundle.DecryptFile(*inputPath, *outputPath, readBundleKey(*keyPath)); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("decrypted evidence bundle: %s\n", *outputPath)
}

func readBundleKey(path string) []byte {
	key, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		fatal(fmt.Sprintf("read bundle encryption key: %v", err))
	}
	if len(key) != 32 {
		fatal("bundle encryption key must contain exactly 32 raw bytes")
	}
	return key
}

func readPublicKey(path, label string) []ed25519.PublicKey {
	data, err := os.ReadFile(filepath.Clean(strings.TrimSpace(path)))
	if err != nil {
		fatal(fmt.Sprintf("read %s public key: %v", label, err))
	}
	if len(data) != ed25519.PublicKeySize {
		fatal(fmt.Sprintf("%s public key must contain %d raw bytes", label, ed25519.PublicKeySize))
	}
	return []ed25519.PublicKey{ed25519.PublicKey(data)}
}

func handleBundleSign(args []string) {
	flags := flag.NewFlagSet("bundle sign", flag.ContinueOnError)
	inputPath := flags.String("input", "", "evidence .tar.gz path")
	privatePath := flags.String("private-key", "", "private key path")
	outputPath := flags.String("output", "", "detached signature JSON path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*inputPath) == "" || strings.TrimSpace(*privatePath) == "" || strings.TrimSpace(*outputPath) == "" {
		fatal("usage: rewind bundle sign --input PATH --private-key PATH --output PATH")
	}
	payload, err := os.ReadFile(filepath.Clean(*inputPath))
	if err != nil {
		fatal(fmt.Sprintf("read evidence bundle: %v", err))
	}
	private, err := os.ReadFile(filepath.Clean(*privatePath))
	if err != nil {
		fatal(fmt.Sprintf("read evidence bundle private key: %v", err))
	}
	signed, err := releasecrypto.Sign(payload, ed25519.PrivateKey(private))
	if err != nil {
		fatal(err.Error())
	}
	if err := writeReleaseJSON(*outputPath, signed); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("signed evidence bundle: %s key_id=%s\n", *outputPath, signed.KeyID)
}
