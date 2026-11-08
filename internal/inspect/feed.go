package inspect

import (
	"embed"
	"io/fs"
	"net/http"
)

// feedFS is the vertical feed (card 275): a page of its own that reads the record
// through /v1 on the same origin, so the browser's credentials for the inspector carry
// to its fetches. It is served as files, not through the page templates, since its
// script is its own file. Until card 276 slice 2 it opens on fixture.js, a generated
// record in the shape of a /v1/timeline answer.
//
//go:embed feed/index.html feed/fixture.js
var feedFS embed.FS

// feedHandler serves the feed's files under /feed/. The directory holds no
// index beyond index.html, so a listing is never shown.
func feedHandler() http.Handler {
	sub, err := fs.Sub(feedFS, "feed")
	if err != nil {
		panic(err) // the embed pattern names the directory; absent means a build mistake
	}
	return http.StripPrefix("/feed/", http.FileServerFS(sub))
}
