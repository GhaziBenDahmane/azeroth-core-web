package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	Addr, AuthDSN, CharactersDSN, WorldDSN string
	AuthDB, CharactersDB, WorldDB          string
	PublicURL, RealmName, RealmAddress     string
	SOAPURL, SOAPUser, SOAPPassword        string
	CookieSecure                           bool
	Expansion                              int
	StartingCredits                        int
	AdminToken                             string
	MockMode                               bool
	RealmID                                int
	GMLevel                                int
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func Load() (Config, error) {
	c := Config{
		Addr:    env("PORTAL_ADDR", ":8080"),
		AuthDSN: os.Getenv("AUTH_DSN"), CharactersDSN: os.Getenv("CHARACTERS_DSN"), WorldDSN: os.Getenv("WORLD_DSN"),
		AuthDB: env("AUTH_DB", "acore_auth"), CharactersDB: env("CHARACTERS_DB", "acore_characters"), WorldDB: env("WORLD_DB", "acore_world"),
		PublicURL: env("PUBLIC_URL", "http://localhost:8080"), RealmName: env("REALM_NAME", "Azeroth"), RealmAddress: env("REALM_ADDRESS", "logon.example.com"),
		SOAPURL: os.Getenv("SOAP_URL"), SOAPUser: os.Getenv("SOAP_USER"), SOAPPassword: os.Getenv("SOAP_PASSWORD"),
		CookieSecure: envBool("COOKIE_SECURE", false), Expansion: envInt("ACCOUNT_EXPANSION", 2),
		StartingCredits: envInt("STARTING_CREDITS", 0), AdminToken: os.Getenv("ADMIN_TOKEN"),
		MockMode: envBool("MOCK_MODE", false),
		RealmID:  envInt("REALM_ID", 1), GMLevel: envInt("GM_LEVEL", 3),
	}
	if c.AuthDSN == "" && !c.MockMode {
		return c, fmt.Errorf("AUTH_DSN is required")
	}
	if c.CharactersDSN == "" {
		c.CharactersDSN = c.AuthDSN
	}
	if c.WorldDSN == "" {
		c.WorldDSN = c.AuthDSN
	}
	for _, name := range []string{c.AuthDB, c.CharactersDB, c.WorldDB} {
		if !identifier.MatchString(name) {
			return c, fmt.Errorf("invalid database name %q", name)
		}
	}
	return c, nil
}
func env(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func envBool(k string, d bool) bool {
	v, e := strconv.ParseBool(os.Getenv(k))
	if e == nil {
		return v
	}
	return d
}
func envInt(k string, d int) int {
	v, e := strconv.Atoi(os.Getenv(k))
	if e == nil {
		return v
	}
	return d
}
