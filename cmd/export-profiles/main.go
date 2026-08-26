// Command export-profiles exports the current hardcoded client profiles
// to a JSON file that can be loaded back via profiles.LoadProfilesFromJSONFile.
//
// Usage:
//
//	go run ./cmd/export-profiles -o profiles.json
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wuliao6688/cloak/profiles"
)

func main() {
	outPath := flag.String("o", "profiles.json", "output JSON file path")
	flag.Parse()

	fmt.Printf("exporting %d profiles to %s...\n", len(profiles.MappedTLSClients), *outPath)

	if err := profiles.ExportProfilesToJSON(*outPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Print summary
	file, err := profiles.LoadProfilesFromJSONFile(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("exported %d profiles (v%s)\n", len(file.Profiles), file.Version)
	fmt.Printf("default profile: %s\n", file.DefaultProfile)
	fmt.Printf("random exclusions: %v\n", file.RandomExclusions)

	// Show a few sizes
	for _, key := range []string{"chrome_150", "chrome_146", "firefox_148", "safari_ios_26_0"} {
		if jp, ok := file.Profiles[key]; ok {
			fmt.Printf("  %-24s ClientHello=%5d bytes\n", key, len(jp.ClientHelloBase64))
		}
	}
}
