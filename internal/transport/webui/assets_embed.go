//go:build webui

package webui

import (
	"embed"
	"io/fs"
)

// The bundle Vite writes. It is gitignored and only exists after a UI build,
// which is why this file is behind a tag: without it, `go build ./...` on a
// fresh clone would fail on a missing embed target rather than on anything
// the caller did wrong.
//
//go:embed all:dist
var bundle embed.FS

func assets() (fs.FS, bool) {
	sub, err := fs.Sub(bundle, "dist")
	if err != nil {
		return nil, false
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil, false
	}
	return sub, true
}
