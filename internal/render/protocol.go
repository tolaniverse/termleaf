package render

import (
	"fmt"
	"os"
	"strings"

	termimg "github.com/blacktop/go-termimg"
)

// ImageMode controls rich terminal image behavior.
type ImageMode string

const (
	ImageAuto   ImageMode = "auto"
	ImagePixels ImageMode = "pixels"
	ImageOff    ImageMode = "off"
)

// ParseImageMode validates a CLI image mode.
func ParseImageMode(value string) (ImageMode, error) {
	mode := ImageMode(strings.ToLower(strings.TrimSpace(value)))
	switch mode {
	case ImageAuto, ImagePixels, ImageOff:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid image mode %q: expected auto, pixels, or off", value)
	}
}

// ImageProtocol is the selected terminal graphics protocol.
type ImageProtocol string

const (
	ProtocolOff        ImageProtocol = "off"
	ProtocolHalfblocks ImageProtocol = "halfblocks"
	ProtocolKitty      ImageProtocol = "kitty"
	ProtocolITerm2     ImageProtocol = "iterm2"
	ProtocolSixel      ImageProtocol = "sixel"
)

// DetectImageProtocol performs conservative environment-based negotiation.
// TERMLEAF_IMAGE_PROTOCOL is an explicit override for testing and recovery.
func DetectImageProtocol(mode ImageMode) ImageProtocol {
	if mode == ImageOff {
		return ProtocolOff
	}
	if mode == ImagePixels {
		return ProtocolHalfblocks
	}
	if override := parseProtocolOverride(os.Getenv("TERMLEAF_IMAGE_PROTOCOL")); override != "" {
		return override
	}
	// Inherited terminal variables are unreliable inside multiplexers. Until
	// native passthrough is actively verified, use cell-safe rendering there.
	term := strings.ToLower(os.Getenv("TERM"))
	if os.Getenv("TMUX") != "" || os.Getenv("STY") != "" || strings.HasPrefix(term, "screen") {
		return ProtocolHalfblocks
	}
	// Kitty's Unicode placeholders participate in the terminal cell grid and
	// are therefore safe for Bubble Tea pagination. iTerm2 and Sixel placement
	// remain available only through an explicit override.
	if termimg.DetectKittyFromEnvironment() {
		return ProtocolKitty
	}
	return ProtocolHalfblocks
}

func parseProtocolOverride(value string) ImageProtocol {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off":
		return ProtocolOff
	case "pixels", "halfblocks":
		return ProtocolHalfblocks
	case "kitty":
		return ProtocolKitty
	case "iterm", "iterm2":
		return ProtocolITerm2
	case "sixel":
		return ProtocolSixel
	default:
		return ""
	}
}

func termimgProtocol(protocol ImageProtocol) termimg.Protocol {
	switch protocol {
	case ProtocolKitty:
		return termimg.Kitty
	case ProtocolITerm2:
		return termimg.ITerm2
	case ProtocolSixel:
		return termimg.Sixel
	default:
		return termimg.Halfblocks
	}
}
