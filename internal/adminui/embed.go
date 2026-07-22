package adminui

import (
	"embed"
	"io/fs"
)

// Assets contains the production build of the React administration panel.
//
//go:embed dist
var assets embed.FS

func Files() (fs.FS, error) {
	return fs.Sub(assets, "dist")
}

func Index() ([]byte, error) {
	return assets.ReadFile("dist/index.html")
}
