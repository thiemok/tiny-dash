package dashboard

import (
	"html/template"
	"io/fs"
)

func loadTemplates(fsys fs.FS) (*template.Template, error) {
	return template.ParseFS(fsys,
		"templates/dashboard.html",
		"templates/partials/clock.html",
		"templates/partials/status.html",
	)
}
