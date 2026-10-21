package def

import (
	"fmt"
	"sync"
	"time"
)

// A fact that names its zone (card 198). A Definition's fact-time rule may read the
// zone a wall clock was said in from the payload itself, through the fact time's
// zone_pointer: the self-report types read source_time's offset there, which names
// one instant even in an hour the zone repeats. A payload that names none is read in
// the Definition's zone, as every fact was before; one that names a zone is read in
// it, and the gate refuses a zone that does not exist, and a wall clock an IANA zone
// named there skips or repeats. Nothing falls back quietly: absent is the one case
// that takes the Definition's zone.

// zones holds every zone loaded by name, an IANA zone or a fixed offset. The names
// are bounded (tzdata's, and offsets within maxOffset to the minute), so it needs no
// eviction, and a batch does not reload tzdata per facet.
var zones sync.Map // string → *time.Location

// loadZone is the location a zone name names: an IANA zone, or a fixed offset
// +HH:MM or -HH:MM named by itself. validZone's rules decide what is one.
func loadZone(name string) (*time.Location, error) {
	if loc, ok := zones.Load(name); ok {
		return loc.(*time.Location), nil
	}
	if err := validZone(name); err != nil {
		return nil, err
	}
	var loc *time.Location
	if off, isOffset, err := offsetSeconds(name); isOffset {
		if err != nil {
			return nil, err
		}
		loc = time.FixedZone(name, off)
	} else {
		var err error
		if loc, err = time.LoadLocation(name); err != nil {
			return nil, fmt.Errorf("%q is not an IANA zone: %v", name, err)
		}
	}
	zones.Store(name, loc)
	return loc, nil
}

// offsetSeconds reads +HH:MM or -HH:MM as seconds east of UTC. isOffset says the text
// is shaped as an offset at all, so the caller can tell a zone name from a bad offset.
func offsetSeconds(s string) (int, bool, error) {
	if s == "" || (s[0] != '+' && s[0] != '-') {
		return 0, false, nil
	}
	t, err := time.Parse("-07:00", s)
	if err != nil {
		return 0, true, fmt.Errorf("%q is not an offset +HH:MM or -HH:MM", s)
	}
	_, off := t.Zone()
	if d := time.Duration(off) * time.Second; d > maxOffset || -d > maxOffset {
		return 0, true, fmt.Errorf("%q is wider than any zone's offset", s)
	}
	return off, true, nil
}

// zoneOf is the zone doc's fact time is read in: the one the payload names at the
// zone pointer, or the Definition's when it names none.
func (d Definition) zoneOf(doc any) (*time.Location, error) {
	p := d.FactTime.ZonePointer
	if p == "" {
		return d.loc, nil
	}
	name, present, err := optionalText(doc, p)
	if err != nil || !present {
		return d.loc, err
	}
	loc, err := loadZone(name)
	if err != nil {
		return nil, fmt.Errorf("%w: zone %s: %v", ErrNotCompliant, p, err)
	}
	return loc, nil
}

// zoneKeepsOffset refuses a fact placed by an offset that the zone it names does not
// keep at that wall clock (card 198). The offset places the fact, so a repeated hour
// is settled by it; but London never showed 01:30 on 29 March 2026, and never sat at
// +05:00, and a writer that sends either has read its clock wrong. It applies when
// the fact time reads the offset beside source_time's zone and that zone is a name;
// a zone given as an offset is the offset itself.
func (d Definition) zoneKeepsOffset(doc any, zone string) error {
	p := d.FactTime.ZonePointer
	if p != "/"+SourceTimeField+"/offset" || zone[0] == '+' || zone[0] == '-' {
		return nil
	}
	offset, present, err := optionalText(doc, p)
	if err != nil || !present {
		return err
	}
	at, err := resolve(doc, d.FactTime.Pointer)
	if err != nil {
		return nil // the fact-time read names a missing clock in its own words
	}
	s, _ := at.(string)
	named, err := loadZone(zone)
	if err != nil {
		return fmt.Errorf("%w: %s zone: %v", ErrNotCompliant, SourceTimeField, err)
	}
	fixed, err := loadZone(offset)
	if err != nil {
		return fmt.Errorf("%w: %s offset: %v", ErrNotCompliant, SourceTimeField, err)
	}
	t, err := time.ParseInLocation(d.FactTime.Layout, s, fixed)
	if err != nil {
		return nil // likewise: the fact-time read refuses a value it cannot parse
	}
	if wall := t.In(fixed).Format(wallLayout); t.In(named).Format(wallLayout) != wall {
		return fmt.Errorf("%w: %s %q is not a wall clock %s shows at offset %s", ErrNotCompliant,
			d.FactTime.Pointer, s, zone, offset)
	}
	return nil
}

// optionalText reads the text at p, if the payload holds any there. A value that is
// there and is not text is refused, not skipped.
func optionalText(doc any, p string) (string, bool, error) {
	v, err := resolve(doc, p)
	if err != nil {
		return "", false, nil // nothing there: the Definition's own rule applies
	}
	s, ok := v.(string)
	if !ok {
		return "", false, fmt.Errorf("%w: %s must be text, not %T", ErrNotCompliant, p, v)
	}
	return s, true, nil
}

// validateZonePointers holds the optional zone pointer to its place: a zoneless
// layout, and a field of source_time. That the schema declares it is checked once the
// schema is compiled.
func (d Definition) validateZonePointers() error {
	p := d.FactTime.ZonePointer
	switch {
	case p == "":
		return nil
	case !zoneless(d.FactTime.Layout):
		return fmt.Errorf("%w: time_semantics.fact_time layout %q carries its own offset, so it takes no zone_pointer",
			ErrBadManifest, d.FactTime.Layout)
	}
	if err := checkPointerSyntax(p); err != nil {
		return fmt.Errorf("%w: time_semantics.fact_time: %v", ErrBadManifest, err)
	}
	// A zone lives in source_time, the field card 197 named, whose shape says what a
	// zone and an offset are; Guard's widening proof leans on that.
	if topToken(p) != SourceTimeField {
		return fmt.Errorf("%w: time_semantics.fact_time reads a zone at %s; a zone and its offset live in %s",
			ErrBadManifest, p, SourceTimeField)
	}
	return nil
}

// declaresOptional checks that the schema declares every field the Definition reads
// without requiring: the fact time's zone pointer and the identity template's
// optional placeholders.
func (d Definition) declaresOptional() error {
	ptrs := append([]string{d.FactTime.ZonePointer}, optionalPointers(d.idParts)...)
	for _, p := range ptrs {
		if p == "" {
			continue
		}
		if err := d.HasPointer(p); err != nil {
			return fmt.Errorf("%w: %v", ErrBadManifest, err)
		}
	}
	return nil
}
