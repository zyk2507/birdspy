package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
)

type Config struct {
	Listen                      string                  `json:"listen"`
	DefaultServer               string                  `json:"default_server"`
	CacheTTLSeconds             int                     `json:"cache_ttl_seconds"`
	CacheMaxEntries             int                     `json:"cache_max_entries"`
	CacheCleanupIntervalSeconds int                     `json:"cache_cleanup_interval_seconds"`
	Reader                      ReaderConfig            `json:"reader"`
	Site                        SiteConfig              `json:"site"`
	Organization                OrganizationConfig      `json:"organization"`
	Servers                     map[string]ServerConfig `json:"servers"`
	Flags                       map[string]Flag         `json:"flags"`
	FlagPatterns                CommunityPatterns       `json:"flag_patterns"`
	BlackHoles                  map[string]string       `json:"black_holes"`
	Invalid                     CommunityFilters        `json:"invalid"`
	Filtered                    CommunityFilters        `json:"filtered"`
	Known                       KnownCommunities        `json:"known"`
}

type ReaderConfig struct {
	Mode              string `json:"mode"`
	TinySamples       bool   `json:"tiny_samples"`
	CommandTimeoutSec int    `json:"command_timeout_seconds"`
}

type SiteConfig struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Build    int    `json:"build"`
}

type OrganizationConfig struct {
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Street    string `json:"street"`
	Town      string `json:"town"`
	IC        string `json:"ic"`
	DIC       string `json:"dic"`
	Email     string `json:"email"`
	Founding  int    `json:"founding"`
}

type ServerConfig struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Socket      string `json:"socket"`
	VersionMask string `json:"version_mask"`
}

type CommunityPatterns struct {
	Communities         map[string]string `json:"communities"`
	LargeCommunities    map[string]string `json:"large_communities"`
	ExtendedCommunities map[string]string `json:"extended_communities"`
}

type CommunityFilters struct {
	Communities         [][]string `json:"communities"`
	LargeCommunities    [][]string `json:"large_communities"`
	ExtendedCommunities [][]string `json:"extended_communities"`
}

type KnownCommunities struct {
	Communities         map[string]CommunityInfo `json:"communities"`
	LargeCommunities    map[string]CommunityInfo `json:"large_communities"`
	ExtendedCommunities map[string]CommunityInfo `json:"extended_communities"`
}

type CommunityInfo struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func defaultConfig() Config {
	return Config{
		Listen:                      ":8080",
		CacheTTLSeconds:             300,
		CacheMaxEntries:             1024,
		CacheCleanupIntervalSeconds: 60,
		Reader: ReaderConfig{
			Mode:              "socket",
			TinySamples:       false,
			CommandTimeoutSec: 180,
		},
		Site: SiteConfig{
			Title:    "BirdSpy",
			Subtitle: "Looking Glass for BIRD Internet Routing Daemon",
			Build:    2026,
		},
		Organization: OrganizationConfig{
			ShortName: "BirdSpy",
			Founding:  2026,
		},
		Servers: map[string]ServerConfig{},
		Flags:   map[string]Flag{},
		FlagPatterns: CommunityPatterns{
			Communities:         map[string]string{},
			LargeCommunities:    map[string]string{},
			ExtendedCommunities: map[string]string{},
		},
		BlackHoles: map[string]string{},
		Invalid: CommunityFilters{
			Communities:         [][]string{},
			LargeCommunities:    [][]string{},
			ExtendedCommunities: [][]string{},
		},
		Filtered: CommunityFilters{
			Communities:         [][]string{},
			LargeCommunities:    [][]string{},
			ExtendedCommunities: [][]string{},
		},
		Known: KnownCommunities{
			Communities:         map[string]CommunityInfo{},
			LargeCommunities:    map[string]CommunityInfo{},
			ExtendedCommunities: map[string]CommunityInfo{},
		},
	}
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.CacheTTLSeconds <= 0 {
		c.CacheTTLSeconds = 300
	}
	if c.CacheMaxEntries <= 0 {
		c.CacheMaxEntries = 1024
	}
	if c.CacheCleanupIntervalSeconds <= 0 {
		c.CacheCleanupIntervalSeconds = 60
	}
	if c.Reader.Mode == "" {
		c.Reader.Mode = "socket"
	}
	if c.Reader.CommandTimeoutSec <= 0 {
		c.Reader.CommandTimeoutSec = 180
	}
	if c.Servers == nil {
		c.Servers = map[string]ServerConfig{}
	}
	if c.BlackHoles == nil {
		c.BlackHoles = map[string]string{}
	}
	if c.Flags == nil {
		c.Flags = map[string]Flag{}
	}
	for id, flag := range c.Flags {
		flag.ID = id
		c.Flags[id] = flag
	}
	if c.DefaultServer == "" {
		for id := range c.Servers {
			c.DefaultServer = id
			break
		}
	}
	if c.FlagPatterns.Communities == nil {
		c.FlagPatterns.Communities = map[string]string{}
	}
	if c.FlagPatterns.LargeCommunities == nil {
		c.FlagPatterns.LargeCommunities = map[string]string{}
	}
	if c.FlagPatterns.ExtendedCommunities == nil {
		c.FlagPatterns.ExtendedCommunities = map[string]string{}
	}
	if c.Known.Communities == nil {
		c.Known.Communities = map[string]CommunityInfo{}
	}
	if c.Known.LargeCommunities == nil {
		c.Known.LargeCommunities = map[string]CommunityInfo{}
	}
	if c.Known.ExtendedCommunities == nil {
		c.Known.ExtendedCommunities = map[string]CommunityInfo{}
	}
}

func (c *Config) validate() error {
	if len(c.Servers) == 0 {
		return errors.New("servers must contain at least one BIRD server")
	}
	if c.DefaultServer == "" {
		return errors.New("default_server is required")
	}
	if _, ok := c.Servers[c.DefaultServer]; !ok {
		return fmt.Errorf("default_server %q is not configured", c.DefaultServer)
	}
	for id, server := range c.Servers {
		if strings.TrimSpace(server.Name) == "" {
			return fmt.Errorf("server %q name is required", id)
		}
		if c.Reader.Mode == "socket" && strings.TrimSpace(server.Socket) == "" {
			return fmt.Errorf("server %q socket is required in socket reader mode", id)
		}
	}
	if c.Reader.Mode != "socket" && c.Reader.Mode != "sample" {
		return fmt.Errorf("reader mode must be socket or sample, got %q", c.Reader.Mode)
	}
	return nil
}

func (c *Config) sortedServerIDs() []string {
	ids := make([]string, 0, len(c.Servers))
	for id := range c.Servers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func normalizePrefix(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("prefix is required")
	}
	if strings.Contains(raw, "/") {
		prefix, err := netipParsePrefix(raw)
		if err != nil {
			return "", err
		}
		return prefix, nil
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return "", fmt.Errorf("%q is not a valid IP prefix", raw)
	}
	if ip.To4() != nil {
		return raw + "/32", nil
	}
	return raw + "/128", nil
}

func netipParsePrefix(raw string) (string, error) {
	prefix, err := netipParse(raw)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid CIDR prefix", raw)
	}
	return prefix, nil
}
