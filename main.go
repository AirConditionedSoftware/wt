package main

import "github.com/piskandar/wt/cmd"

// version is stamped at release time by goreleaser via ldflags.
var version = "dev"

func main() {
	cmd.Execute(version)
}
