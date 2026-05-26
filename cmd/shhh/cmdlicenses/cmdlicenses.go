// Package cmdlicenses implements `shhh licenses` — prints the
// MIT notices for shhh itself and every third-party module shhh
// distributes. Covers MIT's "preserve copyright notice" obligation
// for binary distribution channels (install.sh, release tarballs)
// where users don't have the module source tree at hand.
package cmdlicenses

import (
	_ "embed"
	"fmt"
	"runtime/debug"
)

//go:embed gitleaks-LICENSE.txt
var gitleaksLicense string

// Run prints shhh's MIT notice followed by every embedded
// third-party notice. Currently: shhh + gitleaks. New engines
// added later append their notice to the same flow.
func Run(_ []string) error {
	fmt.Println()
	fmt.Println("shhh — secret-redaction tool for AI coding agents")
	fmt.Println("Copyright (c) 2026 Raphaël — Musubi SASU")
	fmt.Println("Distributed under the MIT License.")
	fmt.Println()
	fmt.Println("=============================================================")
	fmt.Println()
	fmt.Printf("Third-party — gitleaks %s\n", gitleaksVersion())
	fmt.Println("https://github.com/gitleaks/gitleaks")
	fmt.Println()
	fmt.Println(gitleaksLicense)
	return nil
}

func gitleaksVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "v8"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/zricethezav/gitleaks/v8" {
			return dep.Version
		}
	}
	return "v8"
}
