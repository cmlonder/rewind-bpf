package pii

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestScanBytesReturnsHashesWithoutValues(t *testing.T) {
	apiToken := "ghp_" + "synthetic_1234567890abcdef"
	data := []byte("email=alice@example.com ssn=123-45-6789 token=" + apiToken + "\n")
	findings := ScanBytes("fixture.txt", data)
	if len(findings) != 3 {
		t.Fatalf("findings=%+v", findings)
	}
	for _, finding := range findings {
		if strings.Contains(finding.ValueHash, "alice") || strings.Contains(finding.ValueHash, "123-45") {
			t.Fatalf("raw value leaked: %+v", finding)
		}
		if finding.Replacement == "" {
			t.Fatalf("missing replacement: %+v", finding)
		}
	}
}

func TestCustomRuleAndStreamAreRedactedWithoutLeakage(t *testing.T) {
	scanner, err := NewScanner([]RuleConfig{{Kind: "internal_id", Pattern: `ACME-[0-9]{6}`, Replacement: "[REDACTED:internal_id]"}})
	if err != nil {
		t.Fatal(err)
	}
	findings, err := scanner.ScanReader("event", bytes.NewBufferString("id=ACME-123456"), 1024)
	if err != nil || len(findings) != 1 {
		t.Fatalf("findings=%v err=%v", findings, err)
	}
	encoded, _ := json.Marshal(findings)
	if strings.Contains(string(encoded), "ACME-123456") {
		t.Fatal("finding leaked raw value")
	}
	if got := string(scanner.RedactBytes([]byte("id=ACME-123456"))); strings.Contains(got, "123456") {
		t.Fatalf("redacted=%q", got)
	}
}

func TestScannerRejectsOversizedStream(t *testing.T) {
	scanner, _ := NewScanner(nil)
	if _, err := scanner.ScanReader("event", bytes.NewBufferString("0123456789"), 4); err == nil {
		t.Fatal("expected stream limit")
	}
}

func TestRedactBytesMasksMatches(t *testing.T) {
	redacted := string(RedactBytes([]byte("contact alice@example.com")))
	if strings.Contains(redacted, "alice@example.com") || !strings.Contains(redacted, "[REDACTED:email]") {
		t.Fatalf("redacted=%q", redacted)
	}
}

func TestRedactBytesIsSafeForEventMetadata(t *testing.T) {
	apiToken := "ghp_" + "synthetic_1234567890abcdef"
	path := string(RedactBytes([]byte("/workspace/alice@example.com/" + apiToken)))
	if strings.Contains(path, "alice@example.com") || strings.Contains(path, apiToken) {
		t.Fatalf("event metadata leaked raw PII: %q", path)
	}
	if !strings.Contains(path, "[REDACTED:email]") || !strings.Contains(path, "[REDACTED:api_token]") {
		t.Fatalf("event metadata was not redacted: %q", path)
	}
}
