package main

import (
	"encoding/json"
	"io"
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

func compareBase(name1, name2 string) bool {
	n1Parts := strings.Split(name1, ".")
	n2Parts := strings.Split(name2, ".")
	return *(*[2]string)(n1Parts[len(n1Parts)-2:]) == *(*[2]string)(n2Parts[len(n2Parts)-2:])
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
