package main

import (
	"fmt"
	"log"

	"github.com/BurntSushi/toml"
)

type Config struct {
	InfoHost string `toml:"info_host"`
	Hosts    map[string]HostOptions
}

type HostOptions struct {
	SocialReferer  bool `toml:"social_referer"`
	GooglebotUA    bool `toml:"googlebot_ua"`
	GooglebotIP    bool `toml:"googlebot_ip"`
	DisableCookies bool `toml:"disble_cookies"`
	InjectHTML     string `toml:"inject_html"`
}

func parseConfigFile() {
	defaultOptions := HostOptions{
		SocialReferer: true,
		GooglebotUA: true,
		GooglebotIP: true,
		DisableCookies: true,
		InjectHTML: "",
	}

	metaData, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		log.Fatal(err)
	}
	//Apply defaults to unset
	for host, options := range config.Hosts {
		//TODO use reflection or generics
		if !metaData.IsDefined(fmt.Sprintf("%s.social_referer", host)){
			options.SocialReferer = defaultOptions.SocialReferer
		}
		if !metaData.IsDefined(fmt.Sprintf("%s.googlebot_ua", host)){
			options.GooglebotUA = defaultOptions.GooglebotUA
		}
		if !metaData.IsDefined(fmt.Sprintf("%s.googlebot_ip", host)){
			options.GooglebotIP = defaultOptions.GooglebotIP
		}
		if !metaData.IsDefined(fmt.Sprintf("%s.disble_cookies", host)){
			options.DisableCookies = defaultOptions.DisableCookies
		}
		if !metaData.IsDefined(fmt.Sprintf("%s.inject_html", host)){
			options.InjectHTML = defaultOptions.InjectHTML
		}
	}
}
