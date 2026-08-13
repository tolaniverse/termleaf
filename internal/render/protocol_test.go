package render

import "testing"

func TestParseImageMode(t *testing.T) {
	for _, value := range []string{"auto", "pixels", "off", "AUTO"} {
		if _, err := ParseImageMode(value); err != nil {
			t.Errorf("ParseImageMode(%q): %v", value, err)
		}
	}
	if _, err := ParseImageMode("magic"); err == nil {
		t.Fatal("invalid image mode was accepted")
	}
}

func TestDetectImageProtocolModes(t *testing.T) {
	if got := DetectImageProtocol(ImagePixels); got != ProtocolHalfblocks {
		t.Fatalf("pixels protocol = %q", got)
	}
	if got := DetectImageProtocol(ImageOff); got != ProtocolOff {
		t.Fatalf("off protocol = %q", got)
	}
}

func TestDetectImageProtocolOverride(t *testing.T) {
	t.Setenv("TERMLEAF_IMAGE_PROTOCOL", "kitty")
	if got := DetectImageProtocol(ImageAuto); got != ProtocolKitty {
		t.Fatalf("override protocol = %q, want kitty", got)
	}
}

func TestDetectImageProtocolFallsBackInsideTmux(t *testing.T) {
	t.Setenv("TERMLEAF_IMAGE_PROTOCOL", "")
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	t.Setenv("KITTY_WINDOW_ID", "42")
	if got := DetectImageProtocol(ImageAuto); got != ProtocolHalfblocks {
		t.Fatalf("tmux protocol = %q, want halfblocks", got)
	}
}

func TestDetectImageProtocolDoesNotAutoSelectITermOrSixel(t *testing.T) {
	t.Setenv("TERMLEAF_IMAGE_PROTOCOL", "")
	t.Setenv("TMUX", "")
	t.Setenv("STY", "")
	t.Setenv("KITTY_WINDOW_ID", "")
	t.Setenv("WEZTERM_EXECUTABLE", "")
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	if got := DetectImageProtocol(ImageAuto); got != ProtocolHalfblocks {
		t.Fatalf("iTerm auto protocol = %q, want conservative halfblocks", got)
	}
}

func TestDetectImageProtocolConservativeFallback(t *testing.T) {
	for _, key := range []string{"TERMLEAF_IMAGE_PROTOCOL", "KITTY_WINDOW_ID", "WEZTERM_EXECUTABLE", "TERM_PROGRAM", "LC_TERMINAL", "ITERM_SESSION_ID", "TERM_SESSION_ID", "XTERM_VERSION"} {
		t.Setenv(key, "")
	}
	t.Setenv("TERM", "dumb")
	if got := DetectImageProtocol(ImageAuto); got != ProtocolHalfblocks {
		t.Fatalf("fallback protocol = %q, want halfblocks", got)
	}
}
