package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
)

// Config holds all application configuration.
type Config struct {
	Addr   string
	DBPath string
	Secret string
}

// Load reads configuration from defaults, env vars, and CLI flags.
func Load() Config {
	var c Config

	// Defaults
	c.Addr = ":7707"
	c.DBPath = "flagkit.json"
	c.Secret = ""

	// Flags
	flag.StringVar(&c.Addr, "addr", c.Addr, "listen address")
	flag.StringVar(&c.DBPath, "db", c.DBPath, "database file path")
	flag.StringVar(&c.Secret, "secret", c.Secret, "token signing secret (auto-generated if empty)")
	flag.Parse()

	// Env vars override defaults but not flags that were explicitly set
	addrFlag := flag.Lookup("addr")
	dbFlag := flag.Lookup("db")
	secretFlag := flag.Lookup("secret")

	if v := os.Getenv("FLAGKIT_ADDR"); v != "" && addrFlag.Value.String() == addrFlag.DefValue {
		c.Addr = v
	}
	if v := os.Getenv("FLAGKIT_DB"); v != "" && dbFlag.Value.String() == dbFlag.DefValue {
		c.DBPath = v
	}
	if v := os.Getenv("FLAGKIT_SECRET"); v != "" && secretFlag.Value.String() == secretFlag.DefValue {
		c.Secret = v
	}

	// Auto-generate secret if not provided
	if c.Secret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		c.Secret = hex.EncodeToString(b)
	}

	return c
}

// String returns a human-readable config summary.
func (c Config) String() string {
	return fmt.Sprintf("addr=%s db=%s", c.Addr, c.DBPath)
}
