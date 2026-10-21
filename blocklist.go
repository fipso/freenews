package main

import (
	"log"
	"net"
	"os"
	"strings"
)

var blockList = map[string]struct{}{}

func loadBlockList() {
	b, err := os.ReadFile(*blockListPath)
	if err != nil {
		log.Fatal(err)
	}
	blockList = parseBlockList(string(b))
	log.Printf("[*] Loaded %d blocked domains", len(blockList))
}

// parseBlockList accepts plain domain lists, hosts files ("0.0.0.0 domain")
// and simple adblock rules ("||domain^"). CRLF line endings are fine.
func parseBlockList(s string) map[string]struct{} {
	list := map[string]struct{}{}
	for _, line := range strings.Split(s, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		fields := strings.Fields(line)
		domain := fields[0]
		// hosts file format: "<ip> <domain>"
		if len(fields) > 1 && net.ParseIP(fields[0]) != nil {
			domain = fields[1]
		}
		domain = strings.TrimPrefix(domain, "||")
		domain = strings.TrimSuffix(domain, "^")
		domain = strings.ToLower(strings.TrimSuffix(domain, "."))

		if domain == "" || domain == "localhost" || net.ParseIP(domain) != nil {
			continue
		}
		list[domain] = struct{}{}
	}
	return list
}

func isBlocked(host string) bool {
	_, ok := blockList[host]
	return ok
}
