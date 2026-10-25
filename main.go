/*
Copyright © 2026 NAME HERE <EMAIL ADDRESS>
*/
package main

import "github.com/jbspeakr/urbanartcore-bot/cmd"

// Version information (set via ldflags during build)
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	// Pass version info to cmd package
	cmd.SetVersionInfo(version, commit, buildDate)
	cmd.Execute()
}
