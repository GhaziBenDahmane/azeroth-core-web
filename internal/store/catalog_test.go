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
		if len(items) != 11 {
			t.Errorf("%s has %d supply types, want 11", role, len(items))
		}
	}
}

func TestEveryPackageHasCompleteUniqueEquipmentLoadout(t *testing.T) {
	for _, d := range defaultCatalog() {
		items, err := equipmentLoadout(d)
		if err != nil {
			t.Fatalf("%s: %v", d.key, err)
		}
		if len(items) < 11 {
			t.Errorf("%s has only %d non-set equipment items", d.key, len(items))
		}
		slots := map[string]bool{}
		names := map[string]bool{}
		for _, item := range items {
			if item.slot == "" || item.name == "" || item.quantity == 0 {
				t.Errorf("%s has incomplete item: %#v", d.key, item)
			}
			if slots[item.slot] {
				t.Errorf("%s has duplicate slot %q", d.key, item.slot)
			}
			if names[item.name] {
				t.Errorf("%s has duplicate item %q", d.key, item.name)
			}
			slots[item.slot], names[item.name] = true, true
		}
		for _, slot := range []string{"neck", "back", "wrist", "waist", "feet", "finger 1", "finger 2", "trinket 1", "trinket 2"} {
			if !slots[slot] {
				t.Errorf("%s is missing %s", d.key, slot)
			}
		}
		if !slots["two hand"] && !slots["main hand"] {
			t.Errorf("%s has no weapon", d.key)
		}
		if (d.tier == "S6" || d.tier == "S7") && !names["Medallion of the Alliance"] {
			t.Errorf("%s has no faction PvP medallion placeholder", d.key)
		}
	}
}
