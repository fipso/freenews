package main

import (
	"log"

	"github.com/BurntSushi/toml"
)

type Config struct {
	InfoHost    string        `toml:"info_host"`
	UpstreamDNS string        `toml:"upstream_dns"`
	Hosts       []HostOptions `toml:"host"`
}

type HostOptions struct {
	Name            string  `toml:"name"`
	FromGoogleCache *bool   `toml:"from_google_cache,omitempty"`
	SocialReferer   *bool   `toml:"social_referer,omitempty"`
	GooglebotUA     *bool   `toml:"googlebot_ua,omitempty"`
	GooglebotIP     *bool   `toml:"googlebot_ip,omitempty"`
	DisableCookies  *bool   `toml:"disable_cookies,omitempty"`
	DisableJS       *bool   `toml:"disable_js,omitempty"`
	InjectHTML      *string `toml:"inject_html,omitempty"`
}

func parseConfigFile() {
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Fatal(err)
	}
}
