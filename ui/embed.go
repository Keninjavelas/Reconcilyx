package ui

import (
	"embed"
	"io/fs"
)

//go:embed dist
var distFiles embed.FS

// Dist returns the built UI filesystem rooted at ui/dist.
func Dist() (fs.FS, bool) {
	sub, err := fs.Sub(distFiles, "dist")
	if err != nil {
		return nil, false
	}
	return sub, true
}

// Raw returns the rooted UI filesystem for the API server.
func Raw() fs.FS {
	sub, ok := Dist()
	if ok {
		return sub
	}
	return distFiles
}
