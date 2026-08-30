package web

import "testing"

func TestRaidEligibilityRuleValidation(t *testing.T) {
	valid := defaultRaidEligibilityRules()
	if !validRaidEligibilityRules(valid) {
		t.Fatal("default raid eligibility rules must be valid")
	}
	tests := []struct {
		name   string
		mutate func(*raidEligibilityRules)
	}{
		{"empty 10 player roster", func(r *raidEligibilityRules) { r.MinMembers10 = 0 }},
		{"inverted 10 player roster", func(r *raidEligibilityRules) { r.MinMembers10, r.MaxMembers10 = 10, 8 }},
		{"oversized 25 player roster", func(r *raidEligibilityRules) { r.MaxMembers25 = 26 }},
		{"implausibly short duration", func(r *raidEligibilityRules) { r.MinDurationSeconds = 9 }},
		{"inverted duration", func(r *raidEligibilityRules) { r.MinDurationSeconds = r.MaxDurationSeconds }},
		{"unbounded ingestion age", func(r *raidEligibilityRules) { r.MaxEventAgeHours = 24*365 + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rules := valid
			test.mutate(&rules)
			if validRaidEligibilityRules(rules) {
				t.Fatal("invalid raid eligibility rules were accepted")
			}
		})
	}
}
