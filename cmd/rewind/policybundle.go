package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rewindbpf/rewind/internal/policy"
	"github.com/rewindbpf/rewind/internal/policybundle"
)

func handlePolicyKeygen(args []string) {
	flags := flag.NewFlagSet("policy keygen", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	privatePath := flags.String("private", "", "private key path")
	publicPath := flags.String("public", "", "public key path")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || *privatePath == "" || *publicPath == "" {
		fatal("usage: rewind policy keygen --private PATH --public PATH")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		fatal(fmt.Sprintf("generate policy key: %v", err))
	}
	if err := writeSecret(*privatePath, private); err != nil {
		fatal(err.Error())
	}
	if err := writePublic(*publicPath, public); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("generated policy signer keypair: public=%s\n", *publicPath)
}

func handlePolicySign(args []string) {
	if len(args) == 0 {
		fatal("usage: rewind policy sign POLICY --name NAME --version VERSION --private-key PATH --output PATH")
	}
	policyPath := args[0]
	flags := flag.NewFlagSet("policy sign", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	name := flags.String("name", "", "package name")
	version := flags.String("version", "", "package version")
	privatePath := flags.String("private-key", "", "private key path")
	outputPath := flags.String("output", "", "signed bundle output path")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *name == "" || *version == "" || *privatePath == "" || *outputPath == "" {
		fatal("usage: rewind policy sign POLICY --name NAME --version VERSION --private-key PATH --output PATH")
	}
	value, err := policy.Load(policyPath)
	if err != nil {
		fatal(err.Error())
	}
	private, err := os.ReadFile(filepath.Clean(*privatePath))
	if err != nil {
		fatal(fmt.Sprintf("read policy private key: %v", err))
	}
	signed, err := policybundle.Sign(policybundle.Bundle{Name: *name, Version: *version, Policy: value}, ed25519.PrivateKey(private))
	if err != nil {
		fatal(err.Error())
	}
	if err := writeJSON(*outputPath, signed); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("signed policy bundle: %s key_id=%s\n", *outputPath, signed.KeyID)
}

func handlePolicyBundleVerify(args []string) {
	if len(args) == 0 {
		fatal("usage: rewind policy verify BUNDLE --public-key PATH")
	}
	bundlePath := args[0]
	flags := flag.NewFlagSet("policy verify", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	publicPath := flags.String("public-key", "", "trusted public key path")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *publicPath == "" {
		fatal("usage: rewind policy verify BUNDLE --public-key PATH")
	}
	data, err := os.ReadFile(filepath.Clean(bundlePath))
	if err != nil {
		fatal(fmt.Sprintf("read policy bundle: %v", err))
	}
	var signed policybundle.Signed
	if err := json.Unmarshal(data, &signed); err != nil {
		fatal(fmt.Sprintf("decode policy bundle: %v", err))
	}
	public, err := os.ReadFile(filepath.Clean(*publicPath))
	if err != nil {
		fatal(fmt.Sprintf("read policy public key: %v", err))
	}
	bundle, err := policybundle.Verify(signed, ed25519.PublicKey(public))
	if err != nil {
		fatal(err.Error())
	}
	fmt.Printf("policy bundle verified: %s@%s key_id=%s\n", bundle.Name, bundle.Version, signed.KeyID)
}

func writeSecret(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create key directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}
	return nil
}

func writePublic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create public key directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}
	return nil
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create policy output directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create policy output: %w", err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write policy output: %w", err)
	}
	return nil
}
