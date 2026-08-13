package filesystem

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The write cmdlets used to route content through a UTF-8 decode/encode pair
// before it reached the disk. That is lossy twice over: a byte that is not
// valid UTF-8 came back as U+FFFD, and a rune above U+FFFD was rewritten to
// '?'. Both happened silently, so a downloaded archive written with out_file
// arrived corrupt with nothing to say so. These tests pin the fix.

// TestSetContent_BinaryRoundTrip is the case that motivated the fix: bytes read
// off disk and written straight back must be identical.
func TestSetContent_BinaryRoundTrip(t *testing.T) {
	// A gzip header followed by bytes that are not valid UTF-8 in any reading:
	// lone continuation bytes, a bare 0xFF, and an embedded NUL.
	payload := []byte{
		0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xfe, 0xfd, 0x80, 0x81, 0xc0, 0xc1, 0x00,
		0xed, 0xa0, 0x80, 0xf4, 0x90, 0x80, 0x80,
	}

	path := filepath.Join(t.TempDir(), "payload.bin")
	if _, err := setContent(SetContentOptions{
		Path:     path,
		Value:    string(payload),
		Encoding: "utf8",
	}); err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("binary content was not preserved:\n  wrote %d bytes: %x\n  read  %d bytes: %x",
			len(payload), payload, len(got), got)
	}
}

// TestSetContent_AstralPlaneUTF8 covers the other half of the old rune filter:
// characters above U+FFFD are ordinary text, not something to substitute.
func TestSetContent_AstralPlaneUTF8(t *testing.T) {
	const want = "héllo 🎉 日本 𝄞"

	path := filepath.Join(t.TempDir(), "astral.txt")
	if _, err := setContent(SetContentOptions{
		Path:     path,
		Value:    want,
		Encoding: "utf8",
	}); err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(got) != want {
		t.Errorf("expected %q, got %q", want, string(got))
	}
}

// TestSetContent_AstralPlaneUTF16 checks that a real transcode keeps the
// characters the target encoding can represent. UTF-16 spells U+1F389 as a
// surrogate pair; the old code replaced it with '?' before the encoder ever
// saw it.
func TestSetContent_AstralPlaneUTF16(t *testing.T) {
	path := filepath.Join(t.TempDir(), "astral_utf16.txt")
	if _, err := setContent(SetContentOptions{
		Path:     path,
		Value:    "a🎉",
		Encoding: "utf16le",
	}); err != nil {
		t.Fatalf("setContent failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	// 'a', then the surrogate pair D83C DF89, little-endian.
	want := []byte{0x61, 0x00, 0x3c, 0xd8, 0x89, 0xdf}
	if !bytes.Equal(got, want) {
		t.Errorf("expected UTF-16LE surrogate pair %x, got %x", want, got)
	}
}

// TestAppendContent_BinaryRoundTrip covers the append path, which shares the
// encoder with setContent and so shared the bug.
func TestAppendContent_BinaryRoundTrip(t *testing.T) {
	chunk := []byte{0xff, 0x00, 0x80, 0xc3, 0x28}

	path := filepath.Join(t.TempDir(), "appended.bin")
	for range 2 {
		if _, err := appendContent(appendOptions{
			SetContentOptions: SetContentOptions{
				Path:     path,
				Value:    string(chunk),
				Encoding: "utf8",
			},
			Append: true,
		}); err != nil {
			t.Fatalf("appendContent failed: %v", err)
		}
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	// appendContent terminates each write with a newline.
	nl := []byte(getNewLine())
	want := append(append(append([]byte{}, chunk...), nl...), append(append([]byte{}, chunk...), nl...)...)
	if !bytes.Equal(got, want) {
		t.Errorf("appended binary content was not preserved:\n  want %x\n  got  %x", want, got)
	}
}
