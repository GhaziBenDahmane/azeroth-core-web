package store

import (
	"fmt"
	"strings"
)

// loadoutItem is deliberately name-based. The installed AzerothCore world
// database remains the source of truth for entry IDs and client build data.
type loadoutItem struct {
	slot     string
	name     string
	quantity uint32
}

type accessoryProfile struct {
	wrist, waist, feet                               string
	neck, back, finger1, finger2, trinket1, trinket2 string
}

var t8Accessories = map[string]accessoryProfile{
	"strength":        {"Bitter Cold Armguards", "Belt of the Titans", "Battlelord's Plate Boots", "Favor of the Dragon Queen", "Drape of the Faceless General", "Brann's Signet Ring", "Bladebearer's Signet", "Mjolnir Runestone", "Dark Matter"},
	"tank":            {"Mimiron's Inferno Couplings", "Indestructible Plate Girdle", "Spiked Deathdealers", "Shard of the Crystal Forest", "Cloak of the Makers", "The Leviathan's Coil", "Signet of the Earthshaker", "Heart of Iron", "Royal Seal of King Llane"},
	"agility-leather": {"Mechanist's Bindings", "Death-warmed Belt", "Footpads of Silence", "Broach of the Wailing Night", "Drape of Icy Intent", "Brann's Signet Ring", "Loop of the Agile", "Mjolnir Runestone", "Comet's Trail"},
	"agility-mail":    {"Wristguards of the Firetender", "Belt of Dragons", "Boots of Living Scale", "Broach of the Wailing Night", "Drape of Icy Intent", "Brann's Signet Ring", "Loop of the Agile", "Pyrite Infuser", "Comet's Trail"},
	"caster-cloth":    {"Bracers of Unleashed Magic", "Sash of Ancient Power", "Spellslinger's Slippers", "Pendant of Fiery Havoc", "Drape of Mortal Downfall", "Conductive Seal", "Nebula Band", "Scale of Fates", "Flare of the Heavens"},
	"caster-leather":  {"Solar Bindings", "Death-warmed Belt", "Boots of Wintry Endurance", "Pendant of Fiery Havoc", "Drape of Mortal Downfall", "Conductive Seal", "Nebula Band", "Scale of Fates", "Flare of the Heavens"},
	"caster-mail":     {"Frost-bound Chain Bracers", "Blue Belt of Chaos", "Lightning Grounded Boots", "Pendant of Fiery Havoc", "Drape of Mortal Downfall", "Conductive Seal", "Nebula Band", "Scale of Fates", "Flare of the Heavens"},
	"healer-cloth":    {"Grasps of Reason", "Cord of the White Dawn", "Savior's Slippers", "Sapphire Amulet of Renewal", "Drape of the Sullen Goddess", "Ring of the Faithful Servant", "Starshine Signet", "Sif's Remembrance", "Meteorite Crystal"},
	"healer-leather":  {"Bracers of the Broodmother", "Belt of Arctic Life", "Boots of Wintry Endurance", "Sapphire Amulet of Renewal", "Drape of the Sullen Goddess", "Ring of the Faithful Servant", "Starshine Signet", "Sif's Remembrance", "Meteorite Crystal"},
	"healer-mail":     {"Bracers of the Broodmother", "Belt of the Fallen Wyrm", "Greaves of the Rockmender", "Sapphire Amulet of Renewal", "Drape of the Sullen Goddess", "Ring of the Faithful Servant", "Starshine Signet", "Sif's Remembrance", "Meteorite Crystal"},
	"healer-plate":    {"Horologist's Wristguards", "Plate Girdle of Righteousness", "Treads of Destiny", "Sapphire Amulet of Renewal", "Drape of the Sullen Goddess", "Ring of the Faithful Servant", "Starshine Signet", "Sif's Remembrance", "Meteorite Crystal"},
}

var t8Profile = map[string]string{
	"t8-warrior-fury": "strength", "t8-warrior-protection": "tank",
	"t8-paladin-retribution": "strength", "t8-paladin-holy": "healer-plate", "t8-paladin-protection": "tank",
	"t8-hunter-marksmanship": "agility-mail", "t8-rogue-assassination": "agility-leather",
	"t8-priest-shadow": "caster-cloth", "t8-priest-holy": "healer-cloth",
	"t8-death-knight-unholy": "strength", "t8-death-knight-blood": "tank",
	"t8-shaman-enhancement": "agility-mail", "t8-shaman-elemental": "caster-mail", "t8-shaman-restoration": "healer-mail",
	"t8-mage-arcane": "caster-cloth", "t8-warlock-affliction": "caster-cloth",
	"t8-druid-feral": "agility-leather", "t8-druid-balance": "caster-leather", "t8-druid-restoration": "healer-leather",
}

var t8Weapons = map[string][]loadoutItem{
	"t8-warrior-fury":        {{"main hand", "Voldrethar, Dark Blade of Oblivion", 1}, {"off hand", "Dark Edge of Depravity", 1}, {"ranged", "Veranus' Bane", 1}},
	"t8-warrior-protection":  {{"main hand", "Titanguard", 1}, {"off hand", "The Boreal Guard", 1}, {"ranged", "Veranus' Bane", 1}},
	"t8-paladin-retribution": {{"two hand", "Voldrethar, Dark Blade of Oblivion", 1}, {"relic", "Libram of Discord", 1}},
	"t8-paladin-holy":        {{"main hand", "Guiding Star", 1}, {"off hand", "Wisdom's Hold", 1}, {"relic", "Libram of Renewal", 1}},
	"t8-paladin-protection":  {{"main hand", "Titanguard", 1}, {"off hand", "The Boreal Guard", 1}, {"relic", "Libram of the Sacred Shield", 1}},
	"t8-hunter-marksmanship": {{"two hand", "Dark Edge of Depravity", 1}, {"ranged", "Siren's Cry", 1}},
	"t8-rogue-assassination": {{"main hand", "Fang of Oblivion", 1}, {"off hand", "Combatant's Bootblade", 1}, {"ranged", "Rising Sun", 1}},
	"t8-priest-shadow":       {{"main hand", "Constellus", 1}, {"off hand", "Ironmender", 1}, {"ranged", "Scepter of Creation", 1}},
	"t8-priest-holy":         {{"main hand", "Guiding Star", 1}, {"off hand", "Ironmender", 1}, {"ranged", "Nurturing Touch", 1}},
	"t8-death-knight-unholy": {{"two hand", "Voldrethar, Dark Blade of Oblivion", 1}, {"relic", "Sigil of the Vengeful Heart", 1}},
	"t8-death-knight-blood":  {{"two hand", "Lotrafen, Spear of the Damned", 1}, {"relic", "Sigil of Deflection", 1}},
	"t8-shaman-enhancement":  {{"main hand", "Vulmir, the Northern Tempest", 1}, {"off hand", "Caress of Insanity", 1}, {"relic", "Totem of the Dancing Flame", 1}},
	"t8-shaman-elemental":    {{"main hand", "Constellus", 1}, {"off hand", "Pulsing Spellshield", 1}, {"relic", "Totem of Electrifying Wind", 1}},
	"t8-shaman-restoration":  {{"main hand", "Guiding Star", 1}, {"off hand", "Wisdom's Hold", 1}, {"relic", "Steamcaller's Totem", 1}},
	"t8-mage-arcane":         {{"main hand", "Constellus", 1}, {"off hand", "Cosmos", 1}, {"ranged", "Scepter of Creation", 1}},
	"t8-warlock-affliction":  {{"main hand", "Constellus", 1}, {"off hand", "Cosmos", 1}, {"ranged", "Scepter of Creation", 1}},
	"t8-druid-feral":         {{"two hand", "Dark Edge of Depravity", 1}, {"relic", "Idol of the Corruptor", 1}},
	"t8-druid-balance":       {{"main hand", "Constellus", 1}, {"off hand", "Ironmender", 1}, {"relic", "Idol of the Crying Wind", 1}},
	"t8-druid-restoration":   {{"main hand", "Guiding Star", 1}, {"off hand", "Ironmender", 1}, {"relic", "Idol of the Flourishing Life", 1}},
}

var pvpWeapons = map[string][]loadoutItem{
	"warrior-arms":        {{"two hand", "Greatsword", 1}, {"ranged", "War Edge", 1}},
	"paladin-retribution": {{"two hand", "Greatsword", 1}, {"relic", "Libram of Justice", 1}},
	"paladin-holy":        {{"main hand", "Salvation", 1}, {"off hand", "Redoubt", 1}, {"relic", "Libram of Fortitude", 1}},
	"hunter-marksmanship": {{"two hand", "Pike", 1}, {"ranged", "Longbow", 1}},
	"rogue-assassination": {{"main hand", "Shanker", 1}, {"off hand", "Eviscerator", 1}, {"ranged", "War Edge", 1}},
	"priest-shadow":       {{"main hand", "Spellblade", 1}, {"off hand", "Reprieve", 1}, {"ranged", "Piercing Touch", 1}},
	"priest-holy":         {{"main hand", "Salvation", 1}, {"off hand", "Reprieve", 1}, {"ranged", "Baton of Light", 1}},
	"death-knight-unholy": {{"two hand", "Decapitator", 1}, {"relic", "Sigil of Strife", 1}},
	"shaman-enhancement":  {{"main hand", "Slicer", 1}, {"off hand", "Quickblade", 1}, {"relic", "Totem of Survival", 1}},
	"shaman-elemental":    {{"main hand", "Spellblade", 1}, {"off hand", "Barrier", 1}, {"relic", "Totem of Indomitability", 1}},
	"shaman-restoration":  {{"main hand", "Salvation", 1}, {"off hand", "Redoubt", 1}, {"relic", "Totem of the Third Wind", 1}},
	"mage-frost":          {{"main hand", "Spellblade", 1}, {"off hand", "Reprieve", 1}, {"ranged", "Piercing Touch", 1}},
	"warlock-affliction":  {{"main hand", "Spellblade", 1}, {"off hand", "Grimoire", 1}, {"ranged", "Touch of Defeat", 1}},
	"druid-feral":         {{"two hand", "Staff", 1}, {"relic", "Idol of Tenacity", 1}},
	"druid-balance":       {{"main hand", "Spellblade", 1}, {"off hand", "Reprieve", 1}, {"relic", "Idol of Steadfastness", 1}},
	"druid-restoration":   {{"main hand", "Salvation", 1}, {"off hand", "Reprieve", 1}, {"relic", "Idol of Resolve", 1}},
}

func pvpOffpieces(d catalogSet) (wrist, waist, feet string) {
	material := "cloth"
	for _, x := range []string{"Plate", "Scaled", "Dreadplate", "Ornamented"} {
		if strings.HasSuffix(d.prefix, x) {
			material = "plate"
			break
		}
	}
	for _, x := range []string{"Chain", "Linked", "Mail", "Ringmail"} {
		if strings.HasSuffix(d.prefix, x) {
			material = "mail"
			break
		}
	}
	for _, x := range []string{"Leather", "Dragonhide", "Wyrmhide", "Kodohide"} {
		if strings.HasSuffix(d.prefix, x) {
			material = "leather"
			break
		}
	}
	suffix := "Triumph"
	if d.role == "caster" {
		suffix = "Dominance"
	}
	if d.role == "healer" {
		suffix = "Salvation"
	}
	types := map[string][3]string{"plate": {"Bracers", "Girdle", "Greaves"}, "mail": {"Wristguards", "Waistguard", "Sabatons"}, "leather": {"Armwraps", "Belt", "Boots"}, "cloth": {"Cuffs", "Cord", "Treads"}}
	x := types[material]
	if material == "cloth" && d.tier == "S6" {
		x[2] = "Slippers"
	}
	return x[0] + " of " + suffix, x[1] + " of " + suffix, x[2] + " of " + suffix
}

func equipmentLoadout(d catalogSet) ([]loadoutItem, error) {
	if d.tier == "T8" {
		profile, ok := t8Accessories[t8Profile[d.key]]
		if !ok {
			return nil, fmt.Errorf("missing T8 accessory profile")
		}
		items := []loadoutItem{{"neck", profile.neck, 1}, {"back", profile.back, 1}, {"wrist", profile.wrist, 1}, {"waist", profile.waist, 1}, {"feet", profile.feet, 1}, {"finger 1", profile.finger1, 1}, {"finger 2", profile.finger2, 1}, {"trinket 1", profile.trinket1, 1}, {"trinket 2", profile.trinket2, 1}}
		return append(items, t8Weapons[d.key]...), nil
	}
	season := map[string]string{"S6": "Furious", "S7": "Relentless"}[d.tier]
	wrist, waist, feet := pvpOffpieces(d)
	physical := d.role == "strength" || d.role == "agility"
	suffix, ring, secondRing := "Dominance", "Band of Dominance", "Titan-Forged Band of Ascendancy"
	trinket1, trinket2 := "Titan-Forged Rune of Accuracy", "Medallion of the Alliance"
	if physical {
		suffix, ring, secondRing, trinket1 = "Triumph", "Band of Triumph", "Titan-Forged Band of Victory", "Titan-Forged Rune of Cruelty"
	}
	if d.role == "healer" {
		suffix, trinket1 = "Salvation", "Titan-Forged Rune of Alacrity"
	}
	if d.tier == "S7" {
		if physical {
			ring = "Band of Victory"
		} else {
			ring = "Band of Ascendancy"
		}
	}
	prefix := season + " Gladiator's "
	items := []loadoutItem{{"neck", prefix + "Pendant of " + suffix, 1}, {"back", prefix + "Cloak of " + suffix, 1}, {"wrist", prefix + wrist, 1}, {"waist", prefix + waist, 1}, {"feet", prefix + feet, 1}, {"finger 1", prefix + ring, 1}, {"finger 2", secondRing, 1}, {"trinket 1", trinket1, 1}, {"trinket 2", trinket2, 1}}
	weaponKey := lowerKey(d.class + "-" + d.spec)
	for _, weapon := range pvpWeapons[weaponKey] {
		weapon.name = prefix + weapon.name
		items = append(items, weapon)
	}
	return items, nil
}

func lowerKey(value string) string {
	result := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] == ' ' {
			result = append(result, '-')
		} else if value[i] >= 'A' && value[i] <= 'Z' {
			result = append(result, value[i]+32)
		} else {
			result = append(result, value[i])
		}
	}
	return string(result)
}
