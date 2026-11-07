package config

import (
	"fmt"
	"strings"

	"github.com/mattjoyce/ashlar"
)

// Only these nullable sequences change kind before composition. All other nulls
// retain their schema meaning and their original provenance.
func normalizeInspectorSections(root ashlar.Value) (ashlar.Value, error) {
	members := root.Members()
	for i, m := range members {
		if m.Key != "inspector" || m.Value.Kind() != ashlar.KindObject {
			continue
		}
		sections := m.Value.Members()
		for j, section := range sections {
			switch section.Key {
			case "domains", "families", "types":
				if section.Value.IsNull() {
					sections[j].Value = ashlar.ArrayValue(nil, section.Value.Provenance())
				}
			}
		}
		value, err := ashlar.ObjectValue(sections, m.Value.Provenance())
		if err != nil {
			return ashlar.Value{}, err
		}
		members[i].Value = value
	}
	if root.Kind() != ashlar.KindObject {
		return root, nil
	}
	return ashlar.ObjectValue(members, root.Provenance())
}

// Check presence and node kinds before decoding can erase null/missing distinctions.
// The strict decoder rejects unknown fields and scalar type mismatches.
func validateInspectorNodes(root ashlar.Value) error {
	inspector, ok := root.Lookup("inspector")
	if !ok {
		return nil
	}
	if inspector.Kind() != ashlar.KindObject {
		return inspectorNodeError("/inspector", "want mapping")
	}
	for _, section := range inspector.Members() {
		if section.Key == "track_type" && !section.Value.IsNull() {
			if _, ok := section.Value.StringVal(); !ok {
				return inspectorNodeError("/inspector/track_type", "want string or null")
			}
		}
		if section.Key != "domains" && section.Key != "families" && section.Key != "types" {
			continue
		}
		path := "/inspector/" + section.Key
		if section.Value.Kind() != ashlar.KindArray {
			return inspectorNodeError(path, "want sequence")
		}
		for i, entry := range section.Value.Elements() {
			base := fmt.Sprintf("%s/%d", path, i)
			if entry.Kind() != ashlar.KindObject {
				return inspectorNodeError(base, "want mapping")
			}
			required := []string{"id"}
			if section.Key == "types" {
				required = []string{"type", "shape"}
			}
			for _, key := range required {
				v, exists := entry.Lookup(key)
				if !exists || v.IsNull() {
					return inspectorNodeError(base+"/"+key, "required nonnull field")
				}
			}
			for _, key := range []string{"label", "legacy_kind", "legacy_family"} {
				if v, exists := entry.Lookup(key); exists {
					text, isString := v.StringVal()
					if !isString || strings.TrimSpace(text) == "" {
						return inspectorNodeError(base+"/"+key, "want nonblank string")
					}
				}
			}
		}
	}
	return nil
}

func inspectorNodeError(pointer, message string) error {
	return settingError{ashlar.NewError(ErrInvalid, ashlar.Pointer(pointer), ashlar.Origin{}, message, nil)}
}
