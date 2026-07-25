package def

// SetVersionOf exposes the manifest-set digest to the package's external tests, so the
// digest's properties can be probed over synthetic sets without a second copy of the
// hashing rule to drift from this one.
var SetVersionOf = setVersionOf
