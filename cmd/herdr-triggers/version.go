package main

import "runtime/debug"

var version = "dev"

func versionString(linked string, info *debug.BuildInfo) string {
	if linked != "dev" {
		return linked
	}
	if info != nil && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}
