package web

import "testing"

func TestLevel80WeaponSkillsCoverEveryClass(t *testing.T) {
	for _, classID := range []uint8{1, 2, 3, 4, 5, 6, 7, 8, 9, 11} {
		skills, ok := level80WeaponSkills[classID]
		if !ok || len(skills) == 0 {
			t.Fatalf("class %d has no weapon skill profile", classID)
		}
		hasDefense, hasUnarmed := false, false
		for _, skill := range skills {
			hasDefense = hasDefense || skill == 95
			hasUnarmed = hasUnarmed || skill == 162
			if skill != 95 && skill != 162 && weaponProficiencySpells[skill] == 0 {
				t.Errorf("class %d skill %d has no proficiency spell", classID, skill)
			}
		}
		if !hasDefense || !hasUnarmed {
			t.Errorf("class %d must include defense and unarmed", classID)
		}
	}
}

func TestApplyStarterMountFaction(t *testing.T) {
	alliance := []bundleItem{{ItemID: allianceGroundMountItem}, {ItemID: allianceFlyingMountItem}, {ItemID: 41599, Quantity: 4}}
	if err := applyStarterMountFaction(alliance, 1); err != nil {
		t.Fatal(err)
	}
	if alliance[0].ItemID != allianceGroundMountItem || alliance[1].ItemID != allianceFlyingMountItem {
		t.Fatal("alliance mounts were unexpectedly replaced")
	}

	horde := []bundleItem{{ItemID: allianceGroundMountItem}, {ItemID: allianceFlyingMountItem}}
	if err := applyStarterMountFaction(horde, 2); err != nil {
		t.Fatal(err)
	}
	if horde[0].ItemID != hordeGroundMountItem || horde[1].ItemID != hordeFlyingMountItem {
		t.Fatalf("wrong Horde mount replacements: %#v", horde)
	}

	if err := applyStarterMountFaction(horde, 99); err == nil {
		t.Fatal("expected unsupported race to fail")
	}
}

func TestLearnedTrainerSpellsUsesTriggeredSpell(t *testing.T) {
	plain := learnedTrainerSpells(123, [3]uint32{}, [3]uint32{})
	if len(plain) != 1 || plain[0] != 123 {
		t.Fatalf("plain trainer spell = %v", plain)
	}
	triggered := learnedTrainerSpells(456, [3]uint32{0, 36, 36}, [3]uint32{0, 789, 790})
	if len(triggered) != 2 || triggered[0] != 789 || triggered[1] != 790 {
		t.Fatalf("triggered trainer spells = %v", triggered)
	}
}
