package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"strings"
	"testing"
)

func TestProfileValidate(t *testing.T) {
	ok := Profile{"Asha", "Building in healthcare", "A technical co-founder"}
	if err := ok.validate(); err != nil {
		t.Fatal(err)
	}
	for name, p := range map[string]Profile{
		"empty name":   {"", ok.Role, ok.Looking},
		"long name":    {strings.Repeat("a", 51), ok.Role, ok.Looking},
		"no role":      {ok.Name, "", ok.Looking},
		"8-word role":  {ok.Name, "a b c d e f g h", ok.Looking},
		"21-word ask":  {ok.Name, ok.Role, strings.Repeat("w ", 21)},
		"giant 1-word": {ok.Name, strings.Repeat("x", 200), ok.Looking},
	} {
		if p.validate() == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCleanPhoto(t *testing.T) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 40, 60)), nil); err != nil {
		t.Fatal(err)
	}
	out, err := cleanPhoto(buf.Bytes())
	if err != nil || len(out) == 0 {
		t.Fatalf("valid JPEG refused: %v", err)
	}
	if _, err := cleanPhoto([]byte("<svg onload=alert(1)>")); err == nil {
		t.Fatal("non-JPEG accepted")
	}
}
