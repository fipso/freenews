package main

import (
	"log"

	"github.com/BurntSushi/toml"
)

type Config struct {
	InfoHost string        `toml:"info_host"`
	Hosts    []HostOptions `toml:"host"`
}

type HostOptions struct {
	Name           string  `toml:"name"`
	SocialReferer  *bool   `toml:"social_referer,omitempty"`
	GooglebotUA    *bool   `toml:"googlebot_ua,omitempty"`
	GooglebotIP    *bool   `toml:"googlebot_ip,omitempty"`
	DisableCookies *bool   `toml:"disble_cookies,omitempty"`
	InjectHTML     *string `toml:"inject_html,omitempty"`
}

func parseConfigFile() {
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Fatal(err)
	}
}
