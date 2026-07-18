package event

import "testing"

func TestValidEvent(t *testing.T) {
	event := Event{
		RunID:       "run_test",
		PID:         42,
		Operation:   UnlinkAt,
		Path:        "/workspace/src/main.go",
		TimestampNS: 123,
		Decision:    Allow,
		Risk:        High,
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestInvalidEventFieldsAreRejected(t *testing.T) {
	base := Event{
		RunID:       "run_test",
		PID:         42,
		Operation:   Write,
		TimestampNS: 123,
		Decision:    Audit,
		Risk:        Medium,
	}
	cases := []struct {
		name string
		edit func(*Event)
	}{
		{name: "missing run id", edit: func(value *Event) { value.RunID = "" }},
		{name: "missing pid", edit: func(value *Event) { value.PID = 0 }},
		{name: "unknown operation", edit: func(value *Event) { value.Operation = "chmod" }},
		{name: "missing timestamp", edit: func(value *Event) { value.TimestampNS = 0 }},
		{name: "unknown decision", edit: func(value *Event) { value.Decision = "block" }},
		{name: "unknown risk", edit: func(value *Event) { value.Risk = "critical" }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			value := base
			test.edit(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestWireCodesAreStable(t *testing.T) {
	operation, ok := OperationCode(RenameAt2)
	if !ok || operation != OperationCodeRenameAt2 {
		t.Fatalf("operation code = %d, ok=%v", operation, ok)
	}
	decision, ok := DecisionCode(Deny)
	if !ok || decision != DecisionCodeDeny {
		t.Fatalf("decision code = %d, ok=%v", decision, ok)
	}
	risk, ok := RiskCode(High)
	if !ok || risk != RiskCodeHigh {
		t.Fatalf("risk code = %d, ok=%v", risk, ok)
	}
	if _, ok := OperationCode("unsupported"); ok {
		t.Fatal("expected unsupported operation to have no wire code")
	}
}
