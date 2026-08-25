//go:build !webui

package webui

import "io/fs"

// No bundle was compiled in. Build with `-tags webui` after `make web` to
// include one.
func assets() (fs.FS, bool) {
	return nil, false
}
