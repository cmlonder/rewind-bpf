package ebpfload

import "testing"

func TestMakeReadRuleKeyNulTerminatesAndPads(t *testing.T) {
	key, err := makeReadRuleKey("/workspace/.env")
	if err != nil {
		t.Fatal(err)
	}
	if string(key.Path[:len("/workspace/.env")]) != "/workspace/.env" {
		t.Fatalf("key prefix = %q", key.Path[:len("/workspace/.env")])
	}
	if key.Path[len("/workspace/.env")] != 0 {
		t.Fatal("key is not NUL terminated")
	}
}

func TestMakeReadRuleKeyRejectsNulAndOversizedPath(t *testing.T) {
	if _, err := makeReadRuleKey("/workspace/\x00.env"); err == nil {
		t.Fatal("expected NUL error")
	}
	if _, err := makeReadRuleKey("/" + string(make([]byte, 255))); err == nil {
		t.Fatal("expected oversized path error")
	}
}

func TestDecisionCode(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		want uint32
	}{
		{name: "audit", in: "audit", want: 1},
		{name: "deny", in: "deny", want: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := decisionCode(test.in)
			if err != nil || got != test.want {
				t.Fatalf("decisionCode(%q) = %d, %v; want %d", test.in, got, err, test.want)
			}
		})
	}
	if _, err := decisionCode("allow"); err == nil {
		t.Fatal("expected allow to be rejected")
	}
}
