package inspect

import "testing"

// Card 228: a token's stored hash shows as its first eight hex characters and nothing
// else, and anything that is not a hex digest that long shows as nothing.
func TestRedactKeepsEightHexCharacters(t *testing.T) {
	const sum = "8f9c8eaf0123456789abcdef0123456789abcdef0123456789abcdef01234567"
	for _, tt := range []struct{ name, in, want string }{
		{"a digest", sum, "8f9c8eaf"},
		{"nothing", "", ""},
		{"too short", "8f9c8ea", ""},
		{"not hex", "zzzzzzzzzzzz", ""},
		{"upper case is shown as sha256sum prints it", "8F9C8EAF0123", "8f9c8eaf"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := redact(tt.in); got != tt.want {
				t.Errorf("redact(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
