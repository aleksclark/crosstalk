package display

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Backlight controls the PWM backlight via sysfs.
type Backlight struct {
	path string
	max  int
}

// OpenBacklight finds and opens the PWM backlight device.
// Looks in /sys/class/backlight/ for a pwm-backlight entry.
func OpenBacklight() (*Backlight, error) {
	base := "/sys/class/backlight"
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", base, err)
	}

	for _, e := range entries {
		path := filepath.Join(base, e.Name())
		maxRaw, err := os.ReadFile(filepath.Join(path, "max_brightness"))
		if err != nil {
			continue
		}
		max, err := strconv.Atoi(strings.TrimSpace(string(maxRaw)))
		if err != nil || max <= 0 {
			continue
		}
		return &Backlight{path: path, max: max}, nil
	}

	return nil, fmt.Errorf("no backlight device found in %s", base)
}

// SetBrightness sets brightness as a fraction (0.0 = off, 1.0 = max).
func (bl *Backlight) SetBrightness(level float64) error {
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	val := int(level * float64(bl.max))
	return os.WriteFile(
		filepath.Join(bl.path, "brightness"),
		[]byte(strconv.Itoa(val)),
		0644,
	)
}

// Max returns the maximum brightness value.
func (bl *Backlight) Max() int {
	return bl.max
}
