package policycompile

import (
	"testing"

	"github.com/rewindbpf/rewind/internal/manifest"
	"github.com/rewindbpf/rewind/internal/policy"
)

func TestCompileReadExpandsPatternsAgainstManifest(t *testing.T) {
	read := policy.ReadPolicy{
		Mode:  policy.ModeEnforce,
		Deny:  []string{"**/.env"},
		Allow: []string{"**/.env.example"},
	}
	snapshot := manifest.Manifest{Version: 1, Entries: []manifest.Entry{
		{Path: "app/.env", Type: "file"},
		{Path: "app/.env.example", Type: "file"},
		{Path: "app/src", Type: "directory"},
		{Path: "README.md", Type: "file"},
	}}

	result, err := CompileRead(read, "/workspace", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rules) != 1 {
		t.Fatalf("rules = %+v, want one rule", result.Rules)
	}
	if result.Rules[0] != (ReadRule{Path: "/workspace/app/.env", Decision: "deny"}) {
		t.Fatalf("rule = %+v", result.Rules[0])
	}
}

func TestCompileReadOffProducesNoRules(t *testing.T) {
	result, err := CompileRead(policy.ReadPolicy{Mode: policy.ModeOff}, "/workspace", manifest.Manifest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != policy.ModeOff || len(result.Rules) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestCompileReadRejectsOversizedKernelPath(t *testing.T) {
	longName := make([]byte, MaxKernelPathBytes)
	for i := range longName {
		longName[i] = 'a'
	}
	_, err := CompileRead(
		policy.ReadPolicy{Mode: policy.ModeEnforce, Deny: []string{"**"}},
		"/workspace",
		manifest.Manifest{Version: 1, Entries: []manifest.Entry{{Path: string(longName), Type: "file"}}},
	)
	if err == nil {
		t.Fatal("expected oversized kernel path error")
	}
}
