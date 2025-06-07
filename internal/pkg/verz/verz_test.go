package verz

import "testing"

func TestSemver(t *testing.T) {
	Major = "1"
	Minor = "2"
	Build = "3"
	if Semver() != "1.2.3" {
		t.Fatalf("unexpected semver: %s", Semver())
	}
}
