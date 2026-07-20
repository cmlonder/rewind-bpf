package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	releasecrypto "github.com/rewindbpf/rewind/internal/release"
)

func handleRelease(args []string) {
	if len(args) == 0 {
		fatal("usage: rewind release keygen --private PATH --public PATH | rewind release sign --input PATH --private-key PATH --output PATH | rewind release verify --input PATH --signature PATH [--public-key PATH]")
	}
	switch args[0] {
	case "keygen":
		handleReleaseKeygen(args[1:])
	case "sign":
		handleReleaseSign(args[1:])
	case "verify":
		handleReleaseVerify(args[1:])
	default:
		fatal("unknown release command")
	}
}

func handleReleaseKeygen(args []string) {
	flags := flag.NewFlagSet("release keygen", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	privatePath := flags.String("private", "", "private key path")
	publicPath := flags.String("public", "", "public key path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *privatePath == "" || *publicPath == "" {
		fatal("usage: rewind release keygen --private PATH --public PATH")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal(fmt.Sprintf("generate release key: %v", err))
	}
	if err := writeSecret(*privatePath, private); err != nil {
		fatal(err.Error())
	}
	if err := writePublic(*publicPath, public); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("generated release signer keypair: public=%s key_id=%s\n", *publicPath, releasecrypto.KeyID(public))
}

func handleReleaseSign(args []string) {
	flags := flag.NewFlagSet("release sign", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	inputPath := flags.String("input", "", "release payload path, usually SHA256SUMS")
	privatePath := flags.String("private-key", "", "private key path")
	outputPath := flags.String("output", "", "detached signature JSON path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *inputPath == "" || *privatePath == "" || *outputPath == "" {
		fatal("usage: rewind release sign --input PATH --private-key PATH --output PATH")
	}
	payload, err := os.ReadFile(filepath.Clean(*inputPath))
	if err != nil {
		fatal(fmt.Sprintf("read release payload: %v", err))
	}
	private, err := os.ReadFile(filepath.Clean(*privatePath))
	if err != nil {
		fatal(fmt.Sprintf("read release private key: %v", err))
	}
	signed, err := releasecrypto.Sign(payload, ed25519.PrivateKey(private))
	if err != nil {
		fatal(err.Error())
	}
	if err := writeReleaseJSON(*outputPath, signed); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("signed release payload: %s key_id=%s sha256=%s\n", *outputPath, signed.KeyID, signed.PayloadSHA256)
}

func handleReleaseVerify(args []string) {
	flags := flag.NewFlagSet("release verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	inputPath := flags.String("input", "", "release payload path")
	signaturePath := flags.String("signature", "", "detached signature JSON path")
	publicPath := flags.String("public-key", "", "optional pinned trusted public key path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *inputPath == "" || *signaturePath == "" {
		fatal("usage: rewind release verify --input PATH --signature PATH [--public-key PATH]")
	}
	payload, err := os.ReadFile(filepath.Clean(*inputPath))
	if err != nil {
		fatal(fmt.Sprintf("read release payload: %v", err))
	}
	signatureData, err := os.ReadFile(filepath.Clean(*signaturePath))
	if err != nil {
		fatal(fmt.Sprintf("read release signature: %v", err))
	}
	var signed releasecrypto.Signature
	if err := json.Unmarshal(signatureData, &signed); err != nil {
		fatal(fmt.Sprintf("decode release signature: %v", err))
	}
	var trusted ed25519.PublicKey
	if *publicPath != "" {
		trustedData, err := os.ReadFile(filepath.Clean(*publicPath))
		if err != nil {
			fatal(fmt.Sprintf("read release public key: %v", err))
		}
		trusted = ed25519.PublicKey(trustedData)
	}
	if len(trusted) > 0 {
		err = releasecrypto.Verify(payload, signed, trusted)
	} else {
		err = releasecrypto.Verify(payload, signed)
	}
	if err != nil {
		fatal(err.Error())
	}
	trust := "embedded key"
	if len(trusted) > 0 {
		trust = "pinned key"
	}
	fmt.Printf("release signature verified: key_id=%s trust=%s sha256=%s\n", signed.KeyID, trust, signed.PayloadSHA256)
}

func writeReleaseJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create release output directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create release output: %w", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write release output: %w", err)
	}
	return nil
}
