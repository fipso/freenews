package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

//https://stackoverflow.com/questions/41670155/get-public-ip-in-golang
type IP struct {
	Query string
}

func getPublicIP() string {
	req, err := http.Get("http://ip-api.com/json/")
	if err != nil {
		return err.Error()
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err.Error()
	}

	var ip IP
	json.Unmarshal(body, &ip)

	return ip.Query
}

func getHostOptions(host string) *HostOptions {
	for _, entry := range config.Hosts {
		nameParts := strings.Split(entry.Name, ".")
		hostParts := strings.Split(host, ".")
		//log.Println(nameParts, hostParts)
		//Only compare domain + tld. Ignore subdomains
		if *(*[2]string)(nameParts[len(nameParts)-2:]) == *(*[2]string)(hostParts[len(hostParts)-2:]) {
			//log.Println("match", entry)
			return &entry
		}
	}
	return nil
}
