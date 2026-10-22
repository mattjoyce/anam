package policy

import "github.com/mattjoyce/anam/internal/config"

// Declared reports whether an importer in file declares source: one the daemon gives a
// token to, or the workbench. The importers file is the rule's data; the store's part
// of the answer is not policy and stays where it is read.
func Declared(file []config.Principal, source string) bool {
	for _, p := range file {
		if p.Role == config.RoleImporter && p.Source == source {
			return true
		}
	}
	return false
}

// MayRetract reports whether a facet of source may be retracted by hand without asking
// whether the importer's resends reconcile it: true for a source no importer declares.
// For a declared source internal/retract refuses once a snapshot of it has completed,
// since the importer's next complete resend retracts what it no longer holds.
func MayRetract(file []config.Principal, source string) bool {
	return !Declared(file, source)
}

// MayRetire reports whether source may be retired: only once no importer declares it,
// so removing the declaration and retiring the source are two steps that cannot be
// confused.
func MayRetire(file []config.Principal, source string) bool {
	return !Declared(file, source)
}
