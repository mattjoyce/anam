package inspect

import "html/template"

// Build is the identity of the daemon serving the pages, as the frame shows it. Version
// is the string `anam version` and GET /v1/status give (v0.<commits>-<short hash>),
// Commit the full commit it was built from, and Date when it was built. Version is
// empty when the caller gave none, and then the frame shows nothing.
type Build struct {
	Version string
	Commit  string
	Date    string
}

// Title is the hover text beside the version: the commit and the build date, whichever
// are known.
func (b Build) Title() string {
	switch {
	case b.Commit != "" && b.Date != "":
		return "commit " + b.Commit + ", built " + b.Date
	case b.Commit != "":
		return "commit " + b.Commit
	case b.Date != "":
		return "built " + b.Date
	}
	return ""
}

// pagesFor is the templates with the frame's functions answered: the build it shows,
// and whether the importers page exists to link to.
func pagesFor(b Build, importers bool) *template.Template {
	return template.Must(tmpl.Clone()).Funcs(template.FuncMap{
		"build":        func() Build { return b },
		"hasImporters": func() bool { return importers },
	})
}
