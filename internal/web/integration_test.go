package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	"github.com/example/azeroth-portal/internal/srp"
	"github.com/example/azeroth-portal/internal/store"
	_ "github.com/go-sql-driver/mysql"
)

// TestVoteCallbackMatrix proves the provider event ledger, account cooldown,
// and provider-supplied voter-IP cooldown against both supported SQL engines.
func TestVoteCallbackMatrix(t *testing.T) {
	base := os.Getenv("PORTAL_TEST_MYSQL_DSN")
	if base == "" {
		t.Skip("PORTAL_TEST_MYSQL_DSN is not set")
	}
	admin, err := sql.Open("mysql", base+"?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("_%d", time.Now().UnixNano())
	names := []string{"portal_vote_auth" + suffix, "portal_vote_characters" + suffix, "portal_vote_world" + suffix}
	for _, name := range names {
		if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			t.Fatal(err)
		}
		name := name
		t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + name + "`") })
	}
	c := config.Config{
		AuthDSN: base + names[0], CharactersDSN: base + names[1], WorldDSN: base + names[2],
		AuthDB: names[0], CharactersDB: names[1], WorldDB: names[2], RealmKey: "integration", DefaultRealmKey: "integration",
		VoteCallbackSecret: "vote-matrix-secret",
	}
	database, err := store.ConnectForMigration(c)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE account (id INT UNSIGNED PRIMARY KEY,username VARCHAR(32) NOT NULL)`,
		`INSERT INTO account(id,username) VALUES(1,'ONE'),(2,'TWO'),(3,'THREE')`,
		`INSERT INTO portal_vote_sites(realm_key,slug,name,url,reward_credits,cooldown_minutes) VALUES('integration','top','Top','https://vote.example.test',10,720)`,
	} {
		if _, err = database.Auth.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{s: database, c: c}
	post := func(username, eventID, ip string) *httptest.ResponseRecorder {
		t.Helper()
		body := fmt.Sprintf(`{"username":%q,"eventId":%q,"ip":%q}`, username, eventID, ip)
		r := httptest.NewRequest(http.MethodPost, "/api/integrations/votes/top", strings.NewReader(body))
		r.SetPathValue("slug", "top")
		r.Header.Set("Authorization", "Bearer "+c.VoteCallbackSecret)
		w := httptest.NewRecorder()
		s.voteSiteCallback(w, r)
		return w
	}
	if w := post("ONE", "event-1", "203.0.113.7"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"credits":10`) {
		t.Fatalf("first vote: %d %s", w.Code, w.Body.String())
	}
	if w := post("ONE", "event-1", "203.0.113.7"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate event: %d %s", w.Code, w.Body.String())
	}
	if w := post("ONE", "event-2", "203.0.113.8"); w.Code != http.StatusConflict {
		t.Fatalf("account cooldown: %d %s", w.Code, w.Body.String())
	}
	if w := post("TWO", "event-3", "203.0.113.7"); w.Code != http.StatusConflict {
		t.Fatalf("IP cooldown: %d %s", w.Code, w.Body.String())
	}
	if w := post("TWO", "event-4", "203.0.113.8"); w.Code != http.StatusOK {
		t.Fatalf("independent vote: %d %s", w.Code, w.Body.String())
	}
	if w := post("THREE", "event-5", "not-an-ip"); w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid voter IP: %d %s", w.Code, w.Body.String())
	}
	assertWalletBalance(t, database.Auth, 1, 10)
	assertWalletBalance(t, database.Auth, 2, 10)
}

// TestForcedPasswordResetMatrix verifies the staff-required reset invariant
// against both database engines used by CI. A forced account cannot receive a
// new portal session, and consuming the one-time token atomically changes the
// AzerothCore SRP verifier and clears the requirement.
func TestForcedPasswordResetMatrix(t *testing.T) {
	base := os.Getenv("PORTAL_TEST_MYSQL_DSN")
	if base == "" {
		t.Skip("PORTAL_TEST_MYSQL_DSN is not set")
	}
	admin, err := sql.Open("mysql", base+"?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("_%d", time.Now().UnixNano())
	names := []string{"portal_reset_auth" + suffix, "portal_reset_char" + suffix, "portal_reset_world" + suffix}
	for _, name := range names {
		if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			t.Fatal(err)
		}
		name := name
		t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + name + "`") })
	}
	c := config.Config{AuthDSN: base + names[0], CharactersDSN: base + names[1], WorldDSN: base + names[2], AuthDB: names[0], CharactersDB: names[1], WorldDB: names[2], RealmKey: "integration", DefaultRealmKey: "integration", PublicURL: "http://portal.test", PortalName: "Test Portal"}
	database, err := store.ConnectForMigration(c)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE account (id INT UNSIGNED PRIMARY KEY,username VARCHAR(32) NOT NULL,email VARCHAR(255) NOT NULL,salt BINARY(32) NOT NULL,verifier BINARY(32) NOT NULL,locked TINYINT NOT NULL DEFAULT 0)`,
		`CREATE TABLE account_banned (id INT UNSIGNED NOT NULL,active TINYINT NOT NULL DEFAULT 1)`,
	} {
		if _, err = database.Auth.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	salt, verifier, err := srp.Registration("PLAYER", "oldpassword1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Auth.ExecContext(ctx, `INSERT INTO account(id,username,email,salt,verifier) VALUES(1,'PLAYER','player@example.test',?,?)`, salt, verifier); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Auth.ExecContext(ctx, `INSERT INTO portal_forced_password_resets(account_id,actor_account_id,reason) VALUES(1,99,'Compromised credentials')`); err != nil {
		t.Fatal(err)
	}
	server := &Server{s: database, c: c, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	request := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://portal.test"+path, bytes.NewBufferString(body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "http://portal.test")
		w := httptest.NewRecorder()
		server.Handler().ServeHTTP(w, r)
		return w
	}
	if w := request("/api/auth/login", `{"username":"PLAYER","password":"oldpassword1"}`); w.Code != http.StatusForbidden {
		t.Fatalf("forced account login = %d: %s", w.Code, w.Body.String())
	}
	token := hex.EncodeToString(bytes.Repeat([]byte{7}, 32))
	hash := sha256.Sum256([]byte(token))
	if _, err = database.Auth.ExecContext(ctx, `INSERT INTO portal_password_resets(token_hash,account_id,expires_at) VALUES(?,?,?)`, hash[:], 1, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if w := request("/api/auth/password/reset", fmt.Sprintf(`{"token":%q,"password":"newpassword2"}`, token)); w.Code != http.StatusOK {
		t.Fatalf("password reset = %d: %s", w.Code, w.Body.String())
	}
	var remaining int
	if err = database.Auth.QueryRowContext(ctx, `SELECT COUNT(*) FROM portal_forced_password_resets WHERE account_id=1`).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatalf("forced reset marker remains: count=%d err=%v", remaining, err)
	}
	if w := request("/api/auth/login", `{"username":"PLAYER","password":"newpassword2"}`); w.Code != http.StatusOK {
		t.Fatalf("new password login = %d: %s", w.Code, w.Body.String())
	}
}

// TestEventRewardMatrix proves that attendance rewards are atomic and exactly
// once on both supported database engines.
func TestEventRewardMatrix(t *testing.T) {
	base := os.Getenv("PORTAL_TEST_MYSQL_DSN")
	if base == "" {
		t.Skip("PORTAL_TEST_MYSQL_DSN is not set")
	}
	admin, err := sql.Open("mysql", base+"?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("_%d", time.Now().UnixNano())
	names := []string{"portal_event_auth" + suffix, "portal_event_char" + suffix, "portal_event_world" + suffix}
	for _, name := range names {
		if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			t.Fatal(err)
		}
		name := name
		t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + name + "`") })
	}
	c := config.Config{AuthDSN: base + names[0], CharactersDSN: base + names[1], WorldDSN: base + names[2], AuthDB: names[0], CharactersDB: names[1], WorldDB: names[2], RealmKey: "integration", DefaultRealmKey: "integration"}
	database, err := store.ConnectForMigration(c)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	result, err := database.Auth.ExecContext(ctx, `INSERT INTO portal_events(realm_key,title,starts_at,status,signup_enabled,reward_credits) VALUES('integration','Trial of the Crusader',DATE_ADD(NOW(),INTERVAL 1 DAY),'scheduled',1,25)`)
	if err != nil {
		t.Fatal(err)
	}
	eventID, _ := result.LastInsertId()
	if _, err = database.Auth.ExecContext(ctx, `INSERT INTO portal_event_registrations(event_id,account_id,character_guid,status) VALUES(?,?,1,'attended')`, eventID, 7); err != nil {
		t.Fatal(err)
	}
	server := &Server{s: database, c: c}
	req := httptest.NewRequest(http.MethodPost, "/api/admin/events/rewards", nil)
	first := server.rewardEventAccount(req, 99, uint64(eventID), 7, "Verified attendance")
	second := server.rewardEventAccount(req, 99, uint64(eventID), 7, "Verified attendance")
	if first.Status != "awarded" || second.Status != "duplicate" {
		t.Fatalf("unexpected reward results: first=%#v second=%#v", first, second)
	}
	var balance, grants uint32
	if err = database.Auth.QueryRowContext(ctx, `SELECT balance FROM portal_wallets WHERE account_id=7`).Scan(&balance); err != nil {
		t.Fatal(err)
	}
	if err = database.Auth.QueryRowContext(ctx, `SELECT COUNT(*) FROM portal_event_reward_grants WHERE event_id=? AND account_id=7`, eventID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if balance != 25 || grants != 1 {
		t.Fatalf("event reward was not exactly once: balance=%d grants=%d", balance, grants)
	}
}

// TestArmoryMetadataMatrix is run by the MySQL/MariaDB CI matrix. It uses a
// minimal representative subset of AzerothCore's optional world DBC schema.
func TestArmoryMetadataMatrix(t *testing.T) {
	base := os.Getenv("PORTAL_TEST_MYSQL_DSN")
	if base == "" {
		t.Skip("PORTAL_TEST_MYSQL_DSN is not set")
	}
	admin, err := sql.Open("mysql", base+"?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database := fmt.Sprintf("portal_armory_%d", time.Now().UnixNano())
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+database+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + database + "`") })
	db, err := sql.Open("mysql", base+database+"?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`CREATE TABLE talent_dbc (TabID INT NOT NULL,TierID INT NOT NULL,ColumnIndex INT NOT NULL,SpellRank_1 INT NOT NULL,SpellRank_2 INT NOT NULL,SpellRank_3 INT NOT NULL,SpellRank_4 INT NOT NULL,SpellRank_5 INT NOT NULL,SpellRank_6 INT NOT NULL,SpellRank_7 INT NOT NULL,SpellRank_8 INT NOT NULL,SpellRank_9 INT NOT NULL)`,
		`CREATE TABLE talenttab_dbc (ID INT PRIMARY KEY,Name_Lang_enUS VARCHAR(100))`,
		`CREATE TABLE glyphproperties_dbc (ID INT PRIMARY KEY,SpellID INT NOT NULL)`,
		`CREATE TABLE spell_dbc (ID INT PRIMARY KEY,Name_Lang_enUS VARCHAR(100),NameSubtext_Lang_enUS VARCHAR(100),Description_Lang_enUS TEXT,SpellIconID INT UNSIGNED NOT NULL,Effect_1 INT NOT NULL DEFAULT 0,EffectApplyAuraName_1 INT NOT NULL DEFAULT 0)`,
		`CREATE TABLE achievement_dbc (ID INT PRIMARY KEY,Title_Lang_enUS VARCHAR(100),Description_Lang_enUS VARCHAR(200),Category INT NOT NULL,Points INT NOT NULL,IconID INT NOT NULL)`,
		`CREATE TABLE achievement_criteria_dbc (ID INT PRIMARY KEY,Achievement_Id INT NOT NULL,Description_Lang_enUS VARCHAR(200),Quantity BIGINT UNSIGNED NOT NULL)`,
		`CREATE TABLE achievement_category_dbc (ID INT PRIMARY KEY,Parent INT NOT NULL,Name_Lang_enUS VARCHAR(100))`,
		`CREATE TABLE faction_dbc (ID INT PRIMARY KEY,Name_Lang_enUS VARCHAR(100))`,
		`CREATE TABLE char_titles_dbc (ID INT PRIMARY KEY,Mask_ID INT NOT NULL,Name_Lang_enUS VARCHAR(100))`,
		`CREATE TABLE characters (guid INT PRIMARY KEY,knownTitles TEXT NOT NULL)`,
		`CREATE TABLE character_reputation (guid INT NOT NULL,faction INT NOT NULL,standing INT NOT NULL,flags TINYINT NOT NULL)`,
		`CREATE TABLE character_spell (guid INT NOT NULL,spell INT NOT NULL,active TINYINT NOT NULL,disabled TINYINT NOT NULL)`,
		`CREATE TABLE character_achievement_progress (guid INT NOT NULL,criteria INT NOT NULL,counter BIGINT UNSIGNED NOT NULL)`,
		`CREATE TABLE character_skills (guid INT NOT NULL,skill INT NOT NULL,value INT NOT NULL,max INT NOT NULL)`,
		`CREATE TABLE skilllineability_dbc (SkillLine INT NOT NULL,Spell INT NOT NULL,MinSkillLineRank INT NOT NULL)`,
		`CREATE TABLE spellitemenchantment_dbc (ID INT PRIMARY KEY,Name_Lang_enUS VARCHAR(100),Src_ItemID INT NOT NULL)`,
		`INSERT INTO talent_dbc VALUES (383,10,2,53385,0,0,0,0,0,0,0,0)`,
		`INSERT INTO talenttab_dbc VALUES (383,'Retribution')`,
		`INSERT INTO glyphproperties_dbc VALUES (54923,54923)`,
		`INSERT INTO spell_dbc VALUES (53385,'Divine Storm','','An instant weapon attack.',236250,0,0),(54923,'Glyph of Seal of Vengeance','','Increases expertise.',0,0,0),(60025,'Albino Drake','','A swift flying mount.',123,6,78),(55377,'Brilliant Titansteel Helm','','Teaches a blacksmithing recipe.',456,0,0)`,
		`INSERT INTO achievement_dbc VALUES (4584,'The Light of Dawn','Defeat the Lich King on heroic difficulty.',14922,10,0)`,
		`INSERT INTO achievement_criteria_dbc VALUES (9001,4584,'Defeat the Lich King on heroic difficulty.',1)`,
		`INSERT INTO achievement_category_dbc VALUES (168,-1,'Dungeons & Raids'),(14922,168,'Icecrown Citadel')`,
		`INSERT INTO faction_dbc VALUES (1106,'Argent Crusade')`,
		`INSERT INTO char_titles_dbc VALUES (140,1,'the Light of Dawn')`,
		`INSERT INTO characters VALUES (1,'2')`,
		`INSERT INTO character_reputation VALUES (1,1106,42999,1)`,
		`INSERT INTO character_spell VALUES (1,60025,1,0),(1,55377,1,0)`,
		`INSERT INTO character_skills VALUES (1,164,450,450)`,
		`INSERT INTO character_achievement_progress VALUES (1,9001,1)`,
		`INSERT INTO skilllineability_dbc VALUES (164,55377,440)`,
		`INSERT INTO spellitemenchantment_dbc VALUES (3817,'Arcanum of Torment',0),(3637,'Relentless Earthsiege Diamond',41398)`,
	}
	for _, statement := range statements {
		if _, err = db.ExecContext(ctx, statement); err != nil {
			t.Fatalf("fixture statement failed: %v", err)
		}
	}
	c := config.Config{WorldDB: database, CharactersDB: database, RealmKey: "integration"}
	s := &Server{s: &store.Store{World: db, Characters: db, C: c}, c: c}
	talents := []talentInsight{{Group: 0, Spells: []spellInsight{{ID: 53385}}}}
	glyphs := []glyphInsight{{Group: 0, Slot: 0, ID: 54923}}
	achievements := []achievementInsight{{ID: 4584}}
	capabilities := s.enrichArmoryMetadata(ctx, talents, glyphs, achievements)
	if capabilities["dbcMetadata"] != true || !talents[0].PointsKnown || talents[0].Points != 1 || talents[0].Spells[0].Name != "Divine Storm" || talents[0].Trees[0].Name != "Retribution" {
		t.Fatalf("talent metadata not enriched: %#v %#v", talents, capabilities)
	}
	if glyphs[0].Name != "Glyph of Seal of Vengeance" || achievements[0].Name != "The Light of Dawn" || achievements[0].CategoryName != "Icecrown Citadel" || achievements[0].ParentCategoryName != "Dungeons & Raids" {
		t.Fatalf("glyph/achievement metadata not enriched: %#v %#v", glyphs, achievements)
	}
	if !s.loadAchievementCriteria(ctx, 1, achievements) || len(achievements[0].Criteria) != 1 || !achievements[0].Criteria[0].Complete {
		t.Fatalf("achievement criteria not enriched: %#v", achievements)
	}
	items := []armoryItem{{Enchantments: "3817 0 0 0 0 0 3637 0 0"}}
	if !s.enrichItemEnhancements(ctx, items) || len(items[0].Enhancements) != 2 || items[0].Enhancements[1].ItemID != 41398 {
		t.Fatalf("item enhancements not enriched: %#v", items)
	}
	collections := s.loadCharacterCollections(ctx, 1)
	if len(collections.Reputations) != 1 || collections.Reputations[0].Name != "Argent Crusade" || len(collections.Titles) != 1 || len(collections.Mounts) != 1 {
		t.Fatalf("character collections not enriched: %#v", collections)
	}
	professions, available := s.loadProfessionCollections(ctx, 1)
	if !available || len(professions) != 1 || professions[0].Name != "Blacksmithing" || len(professions[0].Recipes) != 1 || professions[0].Recipes[0].Name != "Brilliant Titansteel Helm" {
		t.Fatalf("profession recipes not enriched: %#v available=%v", professions, available)
	}
}

// TestMerchandisingMatrix exercises the SQL used by product variants,
// collections, stock history, coupon policy, and raid eligibility on both
// databases in the CI matrix rather than relying on mocks alone.
func TestMerchandisingMatrix(t *testing.T) {
	base := os.Getenv("PORTAL_TEST_MYSQL_DSN")
	if base == "" {
		t.Skip("PORTAL_TEST_MYSQL_DSN is not set")
	}
	admin, err := sql.Open("mysql", base+"?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("_%d", time.Now().UnixNano())
	names := []string{"portal_merch_auth" + suffix, "portal_merch_char" + suffix, "portal_merch_world" + suffix}
	for _, name := range names {
		if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			t.Fatal(err)
		}
		name := name
		t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + name + "`") })
	}
	c := config.Config{AuthDSN: base + names[0], CharactersDSN: base + names[1], WorldDSN: base + names[2], AuthDB: names[0], CharactersDB: names[1], WorldDB: names[2], RealmKey: "integration", DefaultRealmKey: "integration"}
	database, err := store.ConnectForMigration(c)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = database.World.ExecContext(ctx, `CREATE TABLE item_template(entry INT UNSIGNED PRIMARY KEY,name VARCHAR(100),Quality TINYINT UNSIGNED,ItemLevel SMALLINT UNSIGNED,RequiredLevel TINYINT UNSIGNED,InventoryType TINYINT UNSIGNED,displayid INT UNSIGNED)`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.World.ExecContext(ctx, `INSERT INTO item_template VALUES(100,'Base item',4,245,80,1,10),(101,'Variant item',4,258,80,1,11)`); err != nil {
		t.Fatal(err)
	}
	result, err := database.Auth.ExecContext(ctx, `INSERT INTO portal_products(realm_key,name,description,item_id,quantity,price,category,active,tags,visibility_segment,variant_required) VALUES('integration','Bundle','Test',100,1,100,'Items',1,'raid,tank','all',1)`)
	if err != nil {
		t.Fatal(err)
	}
	productID, _ := result.LastInsertId()
	tx, err := database.Auth.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = saveProductVariants(ctx, tx, uint32(productID), []productVariant{{Name: "Tank", SKU: "tank", PriceAdjustment: 10, Active: true, Items: []bundleItem{{ItemID: 101, Quantity: 2}}}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	server := &Server{s: database, c: c}
	loaded, err := server.loadManagedProduct(ctx, uint32(productID))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Variants) != 1 || loaded.Variants[0].Items[0].Name != "Variant item" || loaded.Tags != "raid,tank" {
		t.Fatalf("unexpected product: %#v", loaded)
	}
	collectionResult, err := database.Auth.ExecContext(ctx, `INSERT INTO portal_shop_collections(realm_key,slug,name,active) VALUES('integration','raid-ready','Raid ready',1)`)
	if err != nil {
		t.Fatal(err)
	}
	collectionID, _ := collectionResult.LastInsertId()
	if _, err = database.Auth.ExecContext(ctx, `INSERT INTO portal_collection_products(collection_id,product_id) VALUES(?,?)`, collectionID, productID); err != nil {
		t.Fatal(err)
	}
	collections, err := server.listShopCollections(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(collections) != 1 || len(collections[0].ProductIDs) != 1 || collections[0].ProductIDs[0] != uint32(productID) {
		t.Fatalf("unexpected collections: %#v", collections)
	}
}
