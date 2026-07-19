package supervisor

import "testing"

func TestValidateRequest(t *testing.T) {
	if err := Validate(Request{}); err == nil {
		t.Fatal("empty action should fail")
	}
	if err := Validate(Request{Action: "status", RunID: "run_1"}); err != nil {
		t.Fatal(err)
	}
}
