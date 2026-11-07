package config

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mattjoyce/ashlar"
	"github.com/mattjoyce/ashlar/yamlconfig"
)

// readConfigFragments confines the graph to the resolved config directory. Paths
// inside application settings still expand from the working directory.
func readConfigFragments(file string) ([]yamlconfig.Fragment, error) {
	if file == "" {
		return nil, nil
	}
	real, err := mustExist(file)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(filepath.Dir(real))
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", real, err)
	}
	defer func() { _ = root.Close() }()
	// The operator named the root file, so its own open error is safe to print in
	// full. Ashlar withholds filesystem errors for include targets only.
	f, err := root.Open(filepath.Base(real))
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", real, err)
	}
	_ = f.Close()
	input := graphInput{fsys: root.FS(), original: make(map[string][]byte)}
	fragments, err := yamlconfig.LoadIncludeGraph(context.Background(), input.fsys, filepath.Base(real), yamlconfig.IncludeOptions{
		Transformer: input.transform,
	})
	if input.err != nil {
		return nil, fmt.Errorf("read config %q: %w: %w", real, ErrInvalid, input.err)
	}
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w: %w", real, ErrInvalid, err)
	}
	return restoreFragments(fragments, input.original)
}

// graphInput adapts only documents Ashlar's graph reader would refuse despite
// Anam's compatibility rules. It never serializes YAML. The original strict tree
// is read again with the graph's include chain before composition, preserving all
// node coordinates, even flow mappings and multiline null tags.
type graphInput struct {
	fsys     fs.FS
	original map[string][]byte
	seen     []fs.FileInfo
	err      error
}

func (g *graphInput) transform(_ context.Context, name string, raw []byte) ([]byte, error) {
	g.original[name] = raw
	info, err := fs.Stat(g.fsys, name)
	if err != nil {
		return nil, err
	}
	for _, previous := range g.seen {
		if os.SameFile(previous, info) {
			g.err = ashlar.NewError(ashlar.ErrIncludeCycle, ashlar.RootPointer,
				ashlar.FileOrigin(ashlar.SourceSpan{Source: name}), "repeated include target", nil)
			return nil, g.err
		}
	}
	g.seen = append(g.seen, info)
	if isBlankYAML(raw) {
		return []byte("{}"), nil
	}
	doc, err := yamlconfig.Read(name, raw)
	if err != nil {
		// Let the graph reader report strict syntax failures with the include chain.
		return raw, nil
	}
	if err := checkIncludePaths(g.fsys, name, doc); err != nil {
		g.err = err
		return nil, err
	}
	include, _ := doc.Lookup("include")
	if doc.IsNull() || include.IsNull() {
		// Null include means no edges. All original members are restored below, after
		// strict duplicate-key, alias and multi-document checks on the original bytes.
		return []byte("{}"), nil
	}
	return raw, nil
}

// Ashlar cleans leading-slash paths before checking confinement. Check traversal
// first too, so /../file cannot silently become /file inside the root. An include
// names a file. Ashlar would read every YAML file under a directory, so a file
// dropped into it could change the daemon's config unseen; a directory is refused.
func checkIncludePaths(fsys fs.FS, name string, doc ashlar.Value) error {
	include, ok := doc.Lookup("include")
	if !ok || include.IsNull() {
		return nil
	}
	values := []ashlar.Value{include}
	if include.Kind() == ashlar.KindArray {
		values = include.Elements()
	}
	for i, value := range values {
		target, ok := value.StringVal()
		if !ok {
			continue // Ashlar reports non-string entries, including null array elements.
		}
		target = strings.ReplaceAll(strings.TrimSpace(target), `\`, "/")
		base := path.Dir(name)
		if strings.HasPrefix(target, "/") {
			base = "."
			target = strings.TrimLeft(target, "/")
		}
		joined := path.Join(base, target)
		pointer := ashlar.NewPointer("include")
		if include.Kind() == ashlar.KindArray {
			pointer = pointer.Index(i)
		}
		if joined == ".." || strings.HasPrefix(joined, "../") {
			return ashlar.NewError(ashlar.ErrComposition, pointer,
				value.Provenance().Effective, "escaping include path", nil)
		}
		// A missing or escaping target is Ashlar's to report; only a directory is ours.
		if info, err := fs.Stat(fsys, joined); err == nil && info.IsDir() {
			return ashlar.NewError(ashlar.ErrComposition, pointer,
				value.Provenance().Effective, "include names a directory, not a file", nil)
		}
	}
	return nil
}

func restoreFragments(fragments []yamlconfig.Fragment, originals map[string][]byte) ([]yamlconfig.Fragment, error) {
	out := make([]yamlconfig.Fragment, 0, len(fragments))
	for _, fragment := range fragments {
		raw := originals[fragment.Path]
		doc, err := readDocument(fragment.Path, raw, fragment.Via...)
		if err != nil {
			return nil, fmt.Errorf("read config: %w: %w", ErrInvalid, err)
		}
		if doc.IsOmitted() {
			continue
		}
		// This is the per-fragment normalization seam, before ResolveFragments checks
		// container kinds. Future schema-specific normalization belongs here too.
		doc, err = withoutInclude(doc)
		if err != nil {
			return nil, err
		}
		doc, err = normalizeInspectorSections(doc)
		if err != nil {
			return nil, err
		}
		fragment.Raw, fragment.Root = raw, doc
		out = append(out, fragment)
	}
	return out, nil
}

func withoutInclude(doc ashlar.Value) (ashlar.Value, error) {
	if doc.Kind() != ashlar.KindObject {
		return doc, nil
	}
	members := doc.Members()
	kept := make([]ashlar.Member, 0, len(members))
	for _, member := range members {
		if member.Key != "include" {
			kept = append(kept, member)
		}
	}
	return ashlar.ObjectValue(kept, doc.Provenance())
}
