package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/bobesa/go-domain-util/domainutil"
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

func compareBase(name1, name2 string) bool {
	return domainutil.Domain(name1) == domainutil.Domain(name2)
}

func getHostOptions(host string) *HostOptions {
	for _, entry := range config.Hosts {
		//Only compare domain + tld. Ignore subdomains
		if compareBase(entry.Name, host) {
			//log.Println("match", entry)
			return &entry
		}
	}
	return nil
}
