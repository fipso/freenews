package main

import (
	"log"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

type Config struct {
	InfoHost    string        `toml:"info_host"`
	UpstreamDNS string        `toml:"upstream_dns"`
	Hosts       []HostOptions `toml:"host"`
}

type HostOptions struct {
	Name        string `toml:"name"`
	FromArchive *bool  `toml:"from_archive,omitempty"`
	// Deprecated: Google shut down its cache in 2024. Treated as from_archive
	FromGoogleCache *bool   `toml:"from_google_cache,omitempty"`
	SocialReferer   *bool   `toml:"social_referer,omitempty"`
	GooglebotUA     *bool   `toml:"googlebot_ua,omitempty"`
	GooglebotIP     *bool   `toml:"googlebot_ip,omitempty"`
	DisableCookies  *bool   `toml:"disable_cookies,omitempty"`
	DisableJS       *bool   `toml:"disable_js,omitempty"`
	InjectHTML      *string `toml:"inject_html,omitempty"`
}

// hostsMu guards config.Hosts, which /addhost appends to while the DNS and
// HTTP servers read it
var hostsMu sync.RWMutex

func parseConfigFile() {
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Fatal(err)
	}
	config.InfoHost = strings.ToLower(config.InfoHost)
	for i := range config.Hosts {
		h := &config.Hosts[i]
		h.Name = strings.ToLower(h.Name)
		if h.FromGoogleCache != nil {
			log.Printf("[*] %s: from_google_cache is deprecated (Google Cache is gone), using from_archive", h.Name)
			if h.FromArchive == nil {
				h.FromArchive = h.FromGoogleCache
			}
		}
	}
}

// enabled reports whether an optional bool option is on, given its default
func enabled(opt *bool, def bool) bool {
	if opt == nil {
		return def
	}
	return *opt
}
