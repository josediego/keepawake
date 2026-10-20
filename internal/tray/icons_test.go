package tray

import (
	"bytes"
	"os"
	"testing"
)

func TestIconsArePNG(t *testing.T) {
	signature := []byte("\x89PNG\r\n\x1a\n")
	icons := map[string][]byte{
		"iconActive":   iconActive,
		"iconInactive": iconInactive,
	}
	for name, icon := range icons {
		if len(icon) == 0 {
			t.Errorf("%s is empty", name)
			continue
		}
		if !bytes.HasPrefix(icon, signature) {
			t.Errorf("%s does not start with the PNG signature", name)
		}
	}
}

func TestIconsMatchAssets(t *testing.T) {
	eye, err := os.ReadFile("../../assets/icons/eye.png")
	if err != nil {
		t.Fatalf("read eye.png: %v", err)
	}
	eyeClosed, err := os.ReadFile("../../assets/icons/eye-closed.png")
	if err != nil {
		t.Fatalf("read eye-closed.png: %v", err)
	}

	if !bytes.Equal(iconActive, eye) {
		t.Errorf("iconActive does not match assets/icons/eye.png")
	}
	if !bytes.Equal(iconInactive, eyeClosed) {
		t.Errorf("iconInactive does not match assets/icons/eye-closed.png")
	}
}
