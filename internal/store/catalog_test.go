package store

import "testing"

func TestDefaultCatalogCoversEveryWotLKClassAndTier(t *testing.T) {
	definitions := defaultCatalog()
	if len(definitions) != 51 {
		t.Fatalf("got %d package definitions, want 51", len(definitions))
	}
	for _, classID := range []uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 11} {
		for _, tier := range []string{"S6", "S7", "T8"} {
			found := false
			for _, d := range definitions {
				if d.classID == classID && d.tier == tier {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("missing class %d %s", classID, tier)
			}
		}
	}
	for role, items := range roleSupplies {
		if len(items) != 6 {
			t.Errorf("%s has %d supply types, want 6", role, len(items))
		}
	}
}
