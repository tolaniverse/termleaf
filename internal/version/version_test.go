package version

import "testing"

func TestCurrentNormalizesReleaseVersion(t *testing.T) {
	previous := Value
	t.Cleanup(func() { Value = previous })
	Value = "1.2.3"
	if got := Current(); got != "v1.2.3" {
		t.Fatalf("Current() = %q, want v1.2.3", got)
	}
}

func TestCurrentDevelopmentBuild(t *testing.T) {
	previous := Value
	t.Cleanup(func() { Value = previous })
	Value = "dev"
	if got := Current(); got != "dev" {
		t.Fatalf("Current() = %q, want dev", got)
	}
}
