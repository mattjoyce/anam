package inspect

import (
	"encoding/xml"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The stdlib's tolerant XML tokenizer handles this server-generated HTML after
// removing raw-text elements. This is deliberately not a general browser parser.
// SVG elements, attributes, nesting and text survive, unlike whole-page goldens
// that also pin whitespace, form tokens, scripts and unrelated navigation.
type parityNode struct {
	Tag      string
	Attr     map[string]string
	Children []*parityNode
	Text     string
	CSS      string
}

var parityRawText = regexp.MustCompile(`(?s)<(script|style)\b[^>]*>.*?</(script|style)>`)
var parityCSSComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

func parseParityHTML(t *testing.T, body string) *parityNode {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(parityRawText.ReplaceAllString(body, "")))
	d.Strict, d.AutoClose, d.Entity = false, xml.HTMLAutoClose, xml.HTMLEntity
	root := &parityNode{}
	for _, block := range regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`).FindAllStringSubmatch(body, -1) {
		root.CSS += block[1]
	}
	stack := []*parityNode{root}
	for {
		token, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("parse rendered HTML: %v", err)
		}
		switch v := token.(type) {
		case xml.StartElement:
			n := &parityNode{Tag: v.Name.Local, Attr: map[string]string{}}
			for _, a := range v.Attr {
				n.Attr[a.Name.Local] = a.Value
			}
			p := stack[len(stack)-1]
			p.Children = append(p.Children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) <= 1 {
				t.Fatalf("unbalanced rendered HTML at %s", v.Name.Local)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			stack[len(stack)-1].Text += string(v)
		}
	}
	return root
}

func (n *parityNode) hasClass(class string) bool {
	return slices.Contains(strings.Fields(n.Attr["class"]), class)
}

func (n *parityNode) find(tag, class string) []*parityNode {
	var out []*parityNode
	for _, child := range n.Children {
		if (tag == "" || child.Tag == tag) && (class == "" || child.hasClass(class)) {
			out = append(out, child)
		}
		out = append(out, child.find(tag, class)...)
	}
	return out
}

func (n *parityNode) one(t *testing.T, tag, class string) *parityNode {
	t.Helper()
	all := n.find(tag, class)
	if len(all) != 1 {
		t.Fatalf("%s.%s count got %d want 1", tag, class, len(all))
	}
	return all[0]
}

func parityText(n *parityNode) string {
	text := n.Text
	for _, c := range n.Children {
		text += parityText(c)
	}
	return strings.Join(strings.Fields(text), " ")
}

// Fixed geometry transcribed from the base's swatches at (8, 8.5), including all
// eight shapes. Neither markPath nor another renderer supplies expected paths.
func parityPaths() map[string]string {
	return map[string]string{
		"circle":   "M3.0 8.5a5.0 5.0 0 1 0 10.0 0a5.0 5.0 0 1 0 -10.0 0z",
		"square":   "M3.8 4.2h8.5v8.5h-8.5z",
		"diamond":  "M8.0 2.8l5.8 5.8l-5.8 5.8l-5.8 -5.8z",
		"tri-up":   "M8.0 3.0L13.0 12.2L3.0 12.2z",
		"tri-down": "M8.0 14.0L13.0 4.8L3.0 4.8z",
		"cross":    "M6.2 3.5L9.8 3.5L9.8 6.8L13.0 6.8L13.0 10.2L9.8 10.2L9.8 13.5L6.2 13.5L6.2 10.2L3.0 10.2L3.0 6.8L6.2 6.8z",
		"star":     "M8.0 2.2L9.6 6.3L13.9 6.6L10.6 9.3L11.7 13.6L8.0 11.2L4.3 13.6L5.4 9.3L2.1 6.6L6.4 6.3z",
		"alert":    "M5.3 0.2h5.4l-1.1 9.4h-3.2zM5.8 12.6h4.3v3.8h-4.3z",
	}
}

var parityPathToken = regexp.MustCompile(`[A-Za-z]|[-+]?[0-9]+(?:\.[0-9]+)?`)

// Translation only removes the layout position. Absolute M/L coordinates shift;
// relative commands and radii stay intact. The 0.11 tolerance covers the two
// independent one-decimal roundings of a sub-lane's translated coordinates.
func parityCheckPath(t *testing.T, name, path, shapeName string, x, y float64) {
	t.Helper()
	want := parityPathToken.FindAllString(parityPaths()[shapeName], -1)
	got := parityPathToken.FindAllString(path, -1)
	if len(want) == 0 || len(got) != len(want) {
		t.Errorf("%s %s path changed, got %s", name, shapeName, path)
		return
	}
	command, arg := "", 0
	for i, w := range want {
		wn, err := strconv.ParseFloat(w, 64)
		if err != nil {
			command, arg = w, 0
			if got[i] != w {
				t.Errorf("%s %s path command %d got %s want %s", name, shapeName, i, got[i], w)
				return
			}
			continue
		}
		gn, err := strconv.ParseFloat(got[i], 64)
		if err != nil {
			t.Errorf("%s path argument %d is %q", name, i, got[i])
			return
		}
		if command == "M" || command == "L" {
			if arg%2 == 0 {
				gn, wn = gn-x, wn-8
			} else {
				gn, wn = gn-y, wn-8.5
			}
		}
		if math.Abs(gn-wn) > 0.11 {
			t.Errorf("%s %s path geometry %d got %.2f want %.2f", name, shapeName, i, gn, wn)
			return
		}
		arg++
	}
}

func parityNumber(t *testing.T, n *parityNode, attr string) float64 {
	t.Helper()
	v, err := strconv.ParseFloat(n.Attr[attr], 64)
	if err != nil {
		t.Fatalf("%s attribute %s: %v", n.Tag, attr, err)
	}
	return v
}

// Class checks alone would miss a stylesheet that fills a hollow path. Pin only
// the paint properties that give these marks their meaning, not the whole CSS.
func parityCheckPaint(t *testing.T, page *parityNode) {
	t.Helper()
	rules := map[string]map[string]string{}
	css := parityCSSComment.ReplaceAllString(page.CSS, "")
	for _, rule := range regexp.MustCompile(`([^{}]+)\{([^{}]*)\}`).FindAllStringSubmatch(css, -1) {
		properties := map[string]string{}
		for _, declaration := range strings.Split(rule[2], ";") {
			key, value, ok := strings.Cut(declaration, ":")
			if ok {
				properties[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
		for _, selector := range strings.Split(rule[1], ",") {
			rules[strings.TrimSpace(selector)] = properties
		}
	}
	for _, selector := range []string{".band .pt path.hollow", ".sw .hollow"} {
		if got := rules[selector]["fill"]; got != "none" {
			t.Errorf("%s hollow fill changed, got %q want none", selector, got)
		}
	}
	for _, hue := range []string{"h0", "h1", "h2", "h3", "h4", "h5", "h6", "h7", "hx"} {
		paint := "var(--" + hue + ")"
		for selector, property := range map[string]string{"." + hue: "fill", "svg path.hollow." + hue: "stroke"} {
			if got := rules[selector][property]; got != paint {
				t.Errorf("%s %s changed, got %q want %s", selector, property, got, paint)
			}
		}
	}
}
