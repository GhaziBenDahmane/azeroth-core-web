package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type catalogSet struct {
	key, class, spec, prefix, anchor, role string
	classID                                uint8
	tier, category                         string
	price                                  uint32
}

type catalogItem struct{ id, quantity uint32 }

const level80StarterGold uint32 = 10000

// These are stable 3.3.5a item IDs from AzerothCore's world database. Alliance
// mounts are stored as placeholders and swapped for Horde equivalents when the
// order is created. Mounts remain tangible items the player can learn in game.
var level80StarterItems = []catalogItem{
	{id: 41599, quantity: 4}, // Frostweave Bag (20 slots)
	{id: 18777, quantity: 1}, // Swift Brown Steed
	{id: 25528, quantity: 1}, // Swift Green Gryphon
}

var pvpCatalog = []struct {
	class, spec, armor, role string
	classID                  uint8
}{
	{"Warrior", "Arms", "Plate", "strength", 1}, {"Paladin", "Retribution", "Scaled", "strength", 2}, {"Paladin", "Holy", "Ornamented", "healer", 2},
	{"Hunter", "Marksmanship", "Chain", "agility", 3}, {"Rogue", "Assassination", "Leather", "agility", 4}, {"Priest", "Shadow", "Satin", "caster", 5}, {"Priest", "Holy", "Mooncloth", "healer", 5},
	{"Death Knight", "Unholy", "Dreadplate", "strength", 6}, {"Shaman", "Enhancement", "Linked", "agility", 7}, {"Shaman", "Elemental", "Mail", "caster", 7}, {"Shaman", "Restoration", "Ringmail", "healer", 7},
	{"Mage", "Frost", "Silk", "caster", 8}, {"Warlock", "Affliction", "Felweave", "caster", 9}, {"Druid", "Feral", "Dragonhide", "agility", 11}, {"Druid", "Balance", "Wyrmhide", "caster", 11}, {"Druid", "Restoration", "Kodohide", "healer", 11},
}

var t8Catalog = []catalogSet{
	{class: "Warrior", spec: "Fury", prefix: "Conqueror's Siegebreaker", anchor: "Battleplate", role: "strength", classID: 1}, {class: "Warrior", spec: "Protection", prefix: "Conqueror's Siegebreaker", anchor: "Breastplate", role: "tank", classID: 1},
	{class: "Paladin", spec: "Retribution", prefix: "Conqueror's Aegis", anchor: "Battleplate", role: "strength", classID: 2}, {class: "Paladin", spec: "Holy", prefix: "Conqueror's Aegis", anchor: "Tunic", role: "healer", classID: 2}, {class: "Paladin", spec: "Protection", prefix: "Conqueror's Aegis", anchor: "Breastplate", role: "tank", classID: 2},
	{class: "Hunter", spec: "Marksmanship", prefix: "Conqueror's Scourgestalker", role: "agility", classID: 3}, {class: "Rogue", spec: "Assassination", prefix: "Conqueror's Terrorblade", role: "agility", classID: 4},
	{class: "Priest", spec: "Shadow", prefix: "Conqueror's", anchor: "Cowl of Sanctification", role: "caster", classID: 5}, {class: "Priest", spec: "Holy", prefix: "Conqueror's", anchor: "Circlet of Sanctification", role: "healer", classID: 5},
	{class: "Death Knight", spec: "Unholy", prefix: "Conqueror's Darkruned", anchor: "Battleplate", role: "strength", classID: 6}, {class: "Death Knight", spec: "Blood", prefix: "Conqueror's Darkruned", anchor: "Chestguard", role: "tank", classID: 6},
	{class: "Shaman", spec: "Enhancement", prefix: "Conqueror's Worldbreaker", anchor: "Faceguard", role: "agility", classID: 7}, {class: "Shaman", spec: "Elemental", prefix: "Conqueror's Worldbreaker", anchor: "Headpiece", role: "caster", classID: 7}, {class: "Shaman", spec: "Restoration", prefix: "Conqueror's Worldbreaker", anchor: "Helm", role: "healer", classID: 7},
	{class: "Mage", spec: "Arcane", prefix: "Conqueror's Kirin Tor", role: "caster", classID: 8}, {class: "Warlock", spec: "Affliction", prefix: "Conqueror's Deathbringer", role: "caster", classID: 9},
	{class: "Druid", spec: "Feral", prefix: "Conqueror's Nightsong", anchor: "Headguard", role: "agility", classID: 11}, {class: "Druid", spec: "Balance", prefix: "Conqueror's Nightsong", anchor: "Headpiece", role: "caster", classID: 11}, {class: "Druid", spec: "Restoration", prefix: "Conqueror's Nightsong", anchor: "Cover", role: "healer", classID: 11},
}

type catalogSupply struct {
	name     string
	quantity uint32
}

var roleSupplies = map[string][]catalogSupply{
	"strength": {{"Bold Cardinal Ruby", 20}, {"Relentless Earthsiege Diamond", 1}, {"Arcanum of Torment", 1}, {"Greater Inscription of the Axe", 1}, {"Icescale Leg Armor", 1}, {"Scroll of Enchant Chest - Powerful Stats", 1}, {"Scroll of Enchant Gloves - Crusher", 1}, {"Scroll of Enchant Weapon - Berserking", 2}, {"Scroll of Enchant Cloak - Major Agility", 1}, {"Scroll of Enchant Bracers - Greater Assault", 1}, {"Scroll of Enchant Boots - Greater Assault", 1}},
	"agility":  {{"Delicate Cardinal Ruby", 20}, {"Relentless Earthsiege Diamond", 1}, {"Arcanum of Torment", 1}, {"Greater Inscription of the Axe", 1}, {"Icescale Leg Armor", 1}, {"Scroll of Enchant Chest - Powerful Stats", 1}, {"Scroll of Enchant Gloves - Crusher", 1}, {"Scroll of Enchant Weapon - Berserking", 2}, {"Scroll of Enchant Cloak - Major Agility", 1}, {"Scroll of Enchant Bracers - Greater Assault", 1}, {"Scroll of Enchant Boots - Greater Assault", 1}},
	"caster":   {{"Runed Cardinal Ruby", 20}, {"Chaotic Skyflare Diamond", 1}, {"Arcanum of Burning Mysteries", 1}, {"Greater Inscription of the Storm", 1}, {"Brilliant Spellthread", 1}, {"Scroll of Enchant Chest - Powerful Stats", 1}, {"Scroll of Enchant Gloves - Exceptional Spellpower", 1}, {"Scroll of Enchant Weapon - Mighty Spellpower", 1}, {"Scroll of Enchant Cloak - Greater Speed", 1}, {"Scroll of Enchant Bracer - Superior Spellpower", 1}, {"Scroll of Enchant Boots - Icewalker", 1}},
	"healer":   {{"Runed Cardinal Ruby", 20}, {"Insightful Earthsiege Diamond", 1}, {"Arcanum of Blissful Mending", 1}, {"Greater Inscription of the Crag", 1}, {"Sapphire Spellthread", 1}, {"Scroll of Enchant Chest - Powerful Stats", 1}, {"Scroll of Enchant Gloves - Exceptional Spellpower", 1}, {"Scroll of Enchant Weapon - Mighty Spellpower", 1}, {"Scroll of Enchant Cloak - Greater Speed", 1}, {"Scroll of Enchant Bracer - Superior Spellpower", 1}, {"Scroll of Enchant Boots - Tuskarr's Vitality", 1}},
	"tank":     {{"Solid Majestic Zircon", 20}, {"Austere Earthsiege Diamond", 1}, {"Arcanum of the Stalwart Protector", 1}, {"Greater Inscription of the Pinnacle", 1}, {"Frosthide Leg Armor", 1}, {"Scroll of Enchant Chest - Super Health", 1}, {"Scroll of Enchant Gloves - Armsman", 1}, {"Scroll of Enchant Weapon - Blood Draining", 1}, {"Scroll of Enchant Cloak - Titanweave", 1}, {"Scroll of Enchant Bracer - Major Stamina", 1}, {"Scroll of Enchant Boots - Tuskarr's Vitality", 1}},
}

func suppliesFor(d catalogSet) []catalogSupply {
	supplies := append([]catalogSupply(nil), roleSupplies[d.role]...)
	if d.tier != "S7" {
		supplies[0].name = map[string]string{"strength": "Bold Scarlet Ruby", "agility": "Delicate Scarlet Ruby", "caster": "Runed Scarlet Ruby", "healer": "Runed Scarlet Ruby", "tank": "Solid Sky Sapphire"}[d.role]
	}
	return supplies
}

func defaultCatalog() []catalogSet {
	out := make([]catalogSet, 0, len(pvpCatalog)*2+len(t8Catalog))
	for _, season := range []struct {
		name  string
		price uint32
	}{{"Furious", 110}, {"Relentless", 155}} {
		for _, x := range pvpCatalog {
			out = append(out, catalogSet{key: strings.ToLower(season.name) + "-" + strings.ToLower(strings.ReplaceAll(x.class+"-"+x.spec, " ", "-")), class: x.class, spec: x.spec, prefix: season.name + " Gladiator's " + x.armor, role: x.role, classID: x.classID, tier: map[string]string{"Furious": "S6", "Relentless": "S7"}[season.name], category: "PvP", price: season.price})
		}
	}
	for i := range t8Catalog {
		x := t8Catalog[i]
		x.key = "t8-" + strings.ToLower(strings.ReplaceAll(x.class+"-"+x.spec, " ", "-"))
		x.tier = "T8"
		x.category = "PvE"
		x.price = 135
		out = append(out, x)
	}
	return out
}

func (s *Store) resolveSet(ctx context.Context, d catalogSet) ([]catalogItem, error) {
	q := fmt.Sprintf("SELECT ItemSet FROM `%s`.item_template WHERE RequiredLevel<=80 AND VerifiedBuild>1 AND ItemSet<>0 AND name LIKE ?", s.C.WorldDB)
	args := []any{d.prefix + "%"}
	if d.anchor != "" {
		q += " AND name LIKE ?"
		args = append(args, "%"+d.anchor+"%")
	}
	q += " ORDER BY ItemLevel DESC LIMIT 1"
	var setID uint32
	if err := s.World.QueryRowContext(ctx, q, args...).Scan(&setID); err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	q = fmt.Sprintf("SELECT entry FROM `%s`.item_template WHERE ItemSet=? AND RequiredLevel<=80 AND VerifiedBuild>1 AND name LIKE ? ORDER BY InventoryType,entry", s.C.WorldDB)
	rows, err := s.World.QueryContext(ctx, q, setID, d.prefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []catalogItem{}
	for rows.Next() {
		var x catalogItem
		if rows.Scan(&x.id) == nil {
			x.quantity = 1
			items = append(items, x)
		}
	}
	if len(items) != 5 {
		return nil, nil
	}
	loadout, err := equipmentLoadout(d)
	if err != nil {
		return nil, err
	}
	for _, gear := range loadout {
		q = fmt.Sprintf("SELECT entry FROM `%s`.item_template WHERE name=? AND RequiredLevel<=80 AND VerifiedBuild>1", s.C.WorldDB)
		if gear.name == "Medallion of the Alliance" {
			// The name is reused by Wrathful's ilvl 264 trinket. S6/S7 use the
			// ilvl 226 version; its Horde counterpart is selected at checkout.
			q += " AND ItemLevel<=226"
		}
		q += " ORDER BY ItemLevel DESC,entry DESC LIMIT 1"
		var id uint32
		if err := s.World.QueryRowContext(ctx, q, gear.name).Scan(&id); err == sql.ErrNoRows {
			return nil, nil
		} else if err != nil {
			return nil, err
		}
		items = append(items, catalogItem{id, gear.quantity})
	}
	for _, supply := range suppliesFor(d) {
		q = fmt.Sprintf("SELECT entry FROM `%s`.item_template WHERE name=? AND VerifiedBuild>1 ORDER BY ItemLevel DESC LIMIT 1", s.C.WorldDB)
		var id uint32
		if s.World.QueryRowContext(ctx, q, supply.name).Scan(&id) == nil {
			items = append(items, catalogItem{id, supply.quantity})
		}
	}
	if len(items) != 5+len(loadout)+len(suppliesFor(d)) {
		return nil, nil
	}
	items = append(items, level80StarterItems...)
	return items, nil
}

func (s *Store) SeedDefaultCatalog(ctx context.Context) (int, error) {
	seeded := 0
	for _, d := range defaultCatalog() {
		seedKey := s.C.RealmKey + ":" + d.key
		items, err := s.resolveSet(ctx, d)
		if err != nil {
			return seeded, fmt.Errorf("resolve %s: %w", d.key, err)
		}
		if len(items) < 5 {
			continue
		}
		name := fmt.Sprintf("%s %s %s Package", d.class, d.spec, d.tier)
		description := fmt.Sprintf("Complete %s level-80 loadout for %s %s with gear, gems, enchants, trained class spells and weapon skills, bags, riding mounts, and 10,000 gold.", d.tier, d.spec, d.class)
		var id int64
		if err = s.Auth.QueryRowContext(ctx, "SELECT id FROM portal_products WHERE seed_key=? AND realm_key=? LIMIT 1", seedKey, s.C.RealmKey).Scan(&id); err == sql.ErrNoRows {
			res, insertErr := s.Auth.ExecContext(ctx, `INSERT INTO portal_products(seed_key,name,description,item_id,quantity,price,category,class_id,tier_label,service_level,gold_amount,active,realm_key) VALUES(?,?,?,0,0,?,?,?,?,80,?,1,?)`, seedKey, name, description, d.price, d.category, d.classID, d.tier, level80StarterGold, s.C.RealmKey)
			if insertErr != nil {
				return seeded, insertErr
			}
			id, err = res.LastInsertId()
		} else if err == nil {
			_, err = s.Auth.ExecContext(ctx, "UPDATE portal_products SET name=?,description=?,category=?,class_id=?,tier_label=?,service_level=80,gold_amount=?,active=1 WHERE id=? AND realm_key=?", name, description, d.category, d.classID, d.tier, level80StarterGold, id, s.C.RealmKey)
		}
		if err != nil {
			return seeded, err
		}
		tx, err := s.Auth.BeginTx(ctx, nil)
		if err != nil {
			return seeded, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM portal_product_items WHERE product_id=?", id); err == nil {
			for _, item := range items {
				if _, err = tx.ExecContext(ctx, "INSERT INTO portal_product_items(product_id,item_id,quantity) VALUES(?,?,?)", id, item.id, item.quantity); err != nil {
					break
				}
			}
		}
		if err != nil {
			tx.Rollback()
			return seeded, err
		}
		if err = tx.Commit(); err != nil {
			return seeded, err
		}
		seeded++
	}
	return seeded, nil
}
