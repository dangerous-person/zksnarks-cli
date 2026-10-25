package media

import (
	"embed"
	"io/fs"
)

//go:embed files
var files embed.FS

var FS, _ = fs.Sub(files, "files")
