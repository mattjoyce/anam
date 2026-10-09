package inspect

import (
	"bytes"
	"encoding/json"
	"strings"
)

// jsonToken is one run of JSON text and its kind: key, str, num, lit or pun, or ""
// for the whitespace between tokens. The template renders each as a classed span and
// escapes the text itself, so no HTML is built here.
type jsonToken struct {
	Class string
	Text  string
}

// highlightJSON splits JSON, indented, into classed tokens: the colour-coding an
// editor gives JSON, done on the server so a payload or a Definition reads at a glance
// with no script. Bytes that are not JSON come back as one plain token, shown as they
// are: verbatim beats absent.
func highlightJSON(raw []byte) []jsonToken {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return []jsonToken{{Text: string(raw)}}
	}
	src := pretty.String()
	var out []jsonToken
	emit := func(class, text string) { out = append(out, jsonToken{Class: class, Text: text}) }
	ws := 0 // start of the pending whitespace run
	flush := func(i int) {
		if i > ws {
			emit("", src[ws:i])
		}
	}
	for i := 0; i < len(src); {
		c := src[i]
		var j int
		var class string
		switch {
		case c == '"':
			j, class = endOfString(src, i), "str"
			if nextNonSpace(src, j) == ':' {
				class = "key"
			}
		case c == '-' || (c >= '0' && c <= '9'):
			j, class = i+1, "num"
			for j < len(src) && strings.IndexByte("0123456789+-.eE", src[j]) >= 0 {
				j++
			}
		case c == 't' || c == 'f' || c == 'n':
			j, class = i, "lit"
			for j < len(src) && src[j] >= 'a' && src[j] <= 'z' {
				j++
			}
		case strings.IndexByte("{}[],:", c) >= 0:
			j, class = i+1, "pun"
		default:
			i++ // whitespace from json.Indent, gathered into one run
			continue
		}
		flush(i)
		emit(class, src[i:j])
		i, ws = j, j
	}
	flush(len(src))
	return out
}

// endOfString returns the index just past the closing quote of the string starting at
// i, honouring backslash escapes.
func endOfString(s string, i int) int {
	for j := i + 1; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return len(s)
}

func nextNonSpace(s string, i int) byte {
	for ; i < len(s); i++ {
		if s[i] != ' ' && s[i] != '\n' && s[i] != '\t' && s[i] != '\r' {
			return s[i]
		}
	}
	return 0
}
