package netns

import "testing"

func TestBuildAllowlistPlanIsReviewable(t *testing.T) {
	plan, err := BuildAllowlistPlan([]string{"api.example.com", "example.com."})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 5 || plan.Domains[1] != "example.com" {
		t.Fatalf("plan=%+v", plan)
	}
}

func TestBuildAllowlistPlanRejectsAmbiguousInput(t *testing.T) {
	if _, err := BuildAllowlistPlan([]string{"https://example.com"}); err == nil {
		t.Fatal("expected invalid domain")
	}
}
