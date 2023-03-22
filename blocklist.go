package main

import (
	"log"
	"os"
	"strings"
)

var blockList []string

func loadBlockList() {
	b, err := os.ReadFile(*blockListPath)
	if err != nil {
		log.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "#") {
			continue
		}
		blockList = append(blockList, line)
	}
}
