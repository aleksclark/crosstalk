package audioctl

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Strict ASCII ALSA card ID: letters, digits, underscore, hyphen. No shell metacharacters.
var alsaCardIDRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// usbIdentity holds resolved USB parent attributes for an ALSA card.
type usbIdentity struct {
	VendorID  string
	ProductID string
	Serial    string
	PathTag   string
	CardName  string
	CardID    string
}

// parseALSARoute extracts CARD id and optional DEV number from hw:/plughw: routes.
// Accepts:
//
//	hw:CARD=Device,DEV=0
//	plughw:CARD=Device,DEV=0
//	hw:CARD=Device
//
// Rejects numeric-only card indexes for durable identity (hw:1, plughw:1,0).
func parseALSARoute(route string) (cardID string, dev int, ok bool) {
	route = strings.TrimSpace(route)
	if route == "" {
		return "", 0, false
	}
	lower := strings.ToLower(route)
	var rest string
	switch {
	case strings.HasPrefix(lower, "plughw:"):
		rest = route[len("plughw:"):]
	case strings.HasPrefix(lower, "hw:"):
		rest = route[len("hw:"):]
	default:
		return "", 0, false
	}
	// Prefer CARD= form.
	parts := strings.Split(rest, ",")
	var foundCard bool
	dev = 0
	for _, p := range parts {
		p = strings.TrimSpace(p)
		up := strings.ToUpper(p)
		if strings.HasPrefix(up, "CARD=") {
			cardID = p[len("CARD="):]
			// Preserve original case from p after CARD=
			eq := strings.IndexByte(p, '=')
			if eq >= 0 {
				cardID = p[eq+1:]
			}
			foundCard = true
		} else if strings.HasPrefix(up, "DEV=") {
			eq := strings.IndexByte(p, '=')
			if eq >= 0 {
				var n int
				if _, err := fmt.Sscanf(p[eq+1:], "%d", &n); err == nil {
					dev = n
				}
			}
		}
	}
	if !foundCard {
		// Numeric card index is not accepted for durable identity.
		return "", 0, false
	}
	if !alsaCardIDRe.MatchString(cardID) {
		return "", 0, false
	}
	return cardID, dev, true
}

func isPipeWireRoute(route string) bool {
	r := strings.TrimSpace(route)
	if r == "" {
		return false
	}
	// ALSA hw/plughw are not PipeWire.
	lower := strings.ToLower(r)
	if strings.HasPrefix(lower, "hw:") || strings.HasPrefix(lower, "plughw:") {
		return false
	}
	// Common PipeWire/Pulse names.
	if strings.HasPrefix(r, "alsa_") || strings.Contains(r, ".monitor") {
		return true
	}
	if strings.Contains(r, "bluez") || strings.HasPrefix(r, "pipewire") {
		return true
	}
	// Anything that is not hw/plughw is treated as non-ALSA backend for this feature.
	return true
}

// resolveUSBIdentity maps an ALSA card ID to its USB parent via sysfs.
func resolveUSBIdentity(sysfsRoot, procCards, cardID string) (usbIdentity, error) {
	if !alsaCardIDRe.MatchString(cardID) {
		return usbIdentity{}, fmt.Errorf("%s: invalid card id", ErrCodeInvalidCommand)
	}
	id := usbIdentity{CardID: cardID}

	// Resolve card index from /proc/asound/cards by id string, or via sysfs id file.
	cardIndex, cardName, err := findCardIndex(sysfsRoot, procCards, cardID)
	if err != nil {
		return id, fmt.Errorf("%s: %w", ErrCodeDeviceNotFound, err)
	}
	id.CardName = cardName

	cardSysfs := filepath.Join(sysfsRoot, "class", "sound", fmt.Sprintf("card%d", cardIndex), "device")
	// Walk up looking for USB device (idVendor present).
	cur, err := filepath.EvalSymlinks(cardSysfs)
	if err != nil {
		// Try without eval.
		cur = cardSysfs
	}
	for i := 0; i < 8; i++ {
		vidPath := filepath.Join(cur, "idVendor")
		if st, err := os.Stat(vidPath); err == nil && !st.IsDir() {
			vid, _ := readTrim(vidPath)
			pid, _ := readTrim(filepath.Join(cur, "idProduct"))
			serial, _ := readTrim(filepath.Join(cur, "serial"))
			// ID_PATH_TAG equivalent: use kernel device path basename chain.
			pathTag := usbPathTag(cur)
			id.VendorID = strings.ToLower(vid)
			id.ProductID = strings.ToLower(pid)
			id.Serial = serial
			id.PathTag = pathTag
			if id.VendorID == "" || id.ProductID == "" {
				return id, fmt.Errorf("%s: missing usb ids", ErrCodeDeviceNotFound)
			}
			return id, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur || parent == "/" || parent == sysfsRoot {
			break
		}
		cur = parent
	}
	return id, fmt.Errorf("%s: no usb parent for card %s", ErrCodeDeviceNotFound, cardID)
}

func findCardIndex(sysfsRoot, procCards, cardID string) (int, string, error) {
	// Prefer sysfs /sys/class/sound/cardN/id
	soundDir := filepath.Join(sysfsRoot, "class", "sound")
	entries, err := os.ReadDir(soundDir)
	if err == nil {
		for _, e := range entries {
			name := e.Name()
			if !strings.HasPrefix(name, "card") {
				continue
			}
			var idx int
			if _, err := fmt.Sscanf(name, "card%d", &idx); err != nil {
				continue
			}
			idPath := filepath.Join(soundDir, name, "id")
			got, err := readTrim(idPath)
			if err != nil {
				continue
			}
			if got == cardID {
				// long name from /proc if available
				longName := lookupCardName(procCards, idx)
				return idx, longName, nil
			}
		}
	}
	// Fallback: parse /proc/asound/cards
	data, err := os.ReadFile(procCards)
	if err != nil {
		return 0, "", fmt.Errorf("read cards: %w", err)
	}
	lines := strings.Split(string(data), "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		// " 0 [Device         ]: USB-Audio - USB Audio Device"
		var idx int
		var idField, rest string
		if _, err := fmt.Sscanf(line, "%d [%s", &idx, &idField); err != nil {
			continue
		}
		// idField may include trailing spaces before ]
		idField = strings.TrimRight(idField, " ]")
		// Better parse:
		lb := strings.IndexByte(line, '[')
		rb := strings.IndexByte(line, ']')
		if lb < 0 || rb < lb {
			continue
		}
		idField = strings.TrimSpace(line[lb+1 : rb])
		if idField != cardID {
			continue
		}
		rest = ""
		if i+1 < len(lines) {
			rest = strings.TrimSpace(lines[i+1])
		}
		// Also try same line after ]:
		if colon := strings.Index(line, "]:"); colon >= 0 {
			rest = strings.TrimSpace(line[colon+2:])
		}
		return idx, rest, nil
	}
	return 0, "", fmt.Errorf("card id %q not found", cardID)
}

func lookupCardName(procCards string, idx int) string {
	data, err := os.ReadFile(procCards)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(line), "%d", &n); err != nil || n != idx {
			continue
		}
		if colon := strings.Index(line, "]:"); colon >= 0 {
			return strings.TrimSpace(line[colon+2:])
		}
	}
	return ""
}

func readTrim(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// usbPathTag builds a stable path tag from the sysfs device path.
// Mirrors udev ID_PATH_TAG style (non-alnum → underscore).
func usbPathTag(devicePath string) string {
	// Use the last meaningful USB path components.
	// e.g. .../usb1/1-1/1-1.2 → platform-ish tag from full path under sysfs.
	p := filepath.Clean(devicePath)
	// Drop leading sysfs root-ish prefixes for stability across chroot tests.
	// Convert to tag: replace non [A-Za-z0-9] with _
	var b strings.Builder
	for _, r := range p {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	tag := b.String()
	// Collapse multiple underscores.
	for strings.Contains(tag, "__") {
		tag = strings.ReplaceAll(tag, "__", "_")
	}
	tag = strings.Trim(tag, "_")
	if len(tag) > 96 {
		tag = tag[len(tag)-96:]
		tag = strings.Trim(tag, "_")
	}
	return tag
}

// escapeUIDComponent percent-encodes characters unsafe for UID components.
func escapeUIDComponent(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", c)
	}
	return b.String()
}

// canonicalUID builds usb:<vid>:<pid>:serial:<esc> or usb:<vid>:<pid>:path:<esc>.
func canonicalUID(id usbIdentity) string {
	vid := strings.ToLower(id.VendorID)
	pid := strings.ToLower(id.ProductID)
	if id.Serial != "" {
		return fmt.Sprintf("usb:%s:%s:serial:%s", vid, pid, escapeUIDComponent(id.Serial))
	}
	return fmt.Sprintf("usb:%s:%s:path:%s", vid, pid, escapeUIDComponent(id.PathTag))
}
