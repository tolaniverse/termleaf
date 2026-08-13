package render

import "testing"

func TestCacheStaysWithinByteBudget(t *testing.T) {
	cache := NewCache(800)
	for _, source := range []string{
		"# One\n\nFirst block.",
		"# Two\n\nSecond block with more words.",
		"# Three\n\nThird block with still more words.",
	} {
		if _, err := cache.Render(source, 40); err != nil {
			t.Fatalf("render cached block: %v", err)
		}
	}
	_, used := cache.Size()
	if used > 800 {
		t.Fatalf("cache uses %d bytes, budget is 800", used)
	}
}
