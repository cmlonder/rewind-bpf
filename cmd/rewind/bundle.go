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
	if args[0] == "publish" {
		handleBundlePublish(args[1:])
		return
	}
	if args[0] != "create" {
		fatal("usage: rewind bundle create --record PATH --output PATH | rewind bundle sign --input PATH --private-key PATH --output PATH | rewind bundle publish --input PATH --endpoint URL --signature PATH | rewind bundle verify --input PATH [--signature PATH --public-key PATH]")
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

func handleBundlePublish(args []string) {
	flags := flag.NewFlagSet("bundle publish", flag.ContinueOnError)
	inputPath := flags.String("input", "", "evidence .tar.gz path")
	endpoint := flags.String("endpoint", "", "HTTPS review endpoint")
	signaturePath := flags.String("signature", "", "detached signature JSON path")
	publicPath := flags.String("public-key", "", "optional pinned public key path")
	tokenPath := flags.String("token-file", "", "optional bearer token file")
	allowInsecure := flags.Bool("allow-insecure-localhost", false, "allow HTTP only for localhost test endpoints")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*inputPath) == "" || strings.TrimSpace(*endpoint) == "" || strings.TrimSpace(*signaturePath) == "" {
		fatal("usage: rewind bundle publish --input PATH --endpoint URL --signature PATH [--public-key PATH --token-file PATH --allow-insecure-localhost]")
	}
	if _, err := evidencebundle.Verify(*inputPath); err != nil {
		fatal(fmt.Sprintf("evidence bundle verification failed: %v", err))
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
	} else {
		err = releasecrypto.Verify(payload, signed)
	}
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
