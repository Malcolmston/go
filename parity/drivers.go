package parity

import (
	"embed"
	"fmt"
)

// The driver programs are embedded so a port repo needs nothing on disk but
// this module: each is written into the oracle's work directory at Open time.
//
//go:embed drivers/*
var driverFS embed.FS

func driverSource(name string) string {
	b, err := driverFS.ReadFile("drivers/" + name)
	if err != nil {
		panic(fmt.Sprintf("parity: missing embedded driver %s: %v", name, err))
	}
	return string(b)
}
