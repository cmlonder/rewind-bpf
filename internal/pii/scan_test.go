package pii

import (
	"strings"
	"testing"
)

func TestScanBytesReturnsHashesWithoutValues(t *testing.T) {
	data := []byte("email=alice@example.com ssn=123-45-6789 token=ghp_1234567890abcdef\n")
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

func TestRedactBytesMasksMatches(t *testing.T) {
	redacted := string(RedactBytes([]byte("contact alice@example.com")))
	if strings.Contains(redacted, "alice@example.com") || !strings.Contains(redacted, "[REDACTED:email]") {
		t.Fatalf("redacted=%q", redacted)
	}
}
