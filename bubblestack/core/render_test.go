package core

import (
	"strings"
	"testing"
)

func TestTruncLeft(t *testing.T) {
	if got := TruncLeft("abc", 5); got != "abc" {
		t.Errorf("short string should pass through, got %q", got)
	}
	// Keeps the right (most informative) end, prefixed with "…".
	if got := TruncLeft("abcdefghij", 5); got != "…ghij" {
		t.Errorf("TruncLeft = %q, want …ghij", got)
	}
	// max < 4 is clamped to 4; "abcd" fits.
	if got := TruncLeft("abcd", 2); got != "abcd" {
		t.Errorf("max clamp: got %q, want abcd", got)
	}
}

func TestHeaderInnerWidthAndConfirmWidth(t *testing.T) {
	if got := HeaderInnerWidth(100); got != 96 {
		t.Errorf("HeaderInnerWidth(100) = %d, want 96", got)
	}
	if got := HeaderInnerWidth(10); got != 20 {
		t.Errorf("HeaderInnerWidth floor = %d, want 20", got)
	}

	sh := NewShared(nil)
	sh.width = 100
	if got := sh.ConfirmWidth(); got != 90 {
		t.Errorf("ConfirmWidth(100) = %d, want 90", got)
	}
	sh.width = 5
	if got := sh.ConfirmWidth(); got != 24 {
		t.Errorf("ConfirmWidth floor = %d, want 24", got)
	}
}

func TestHardWrap(t *testing.T) {
	in := "aaaaaaaaaabbbbbbbbbb" // 20 runes
	out := HardWrap(in, 8)
	for _, line := range strings.Split(out, "\n") {
		if len([]rune(line)) > 8 {
			t.Errorf("line exceeds width: %q", line)
		}
	}
	if strings.ReplaceAll(out, "\n", "") != in {
		t.Errorf("HardWrap should preserve content, got %q", out)
	}
	// width < 8 is clamped to 8.
	if got := HardWrap("abcdefghij", 4); !strings.Contains(got, "\n") || len([]rune(strings.SplitN(got, "\n", 2)[0])) != 8 {
		t.Errorf("width clamp: got %q", got)
	}
	// No wrap when the string fits.
	if got := HardWrap("short", 8); got != "short" {
		t.Errorf("no-wrap case: got %q", got)
	}
}

func TestBlanks(t *testing.T) {
	if got := Blanks(0); got != "" {
		t.Errorf("Blanks(0) = %q, want empty", got)
	}
	if got := Blanks(1); got != "" {
		t.Errorf("Blanks(1) = %q, want empty (single line, no newline)", got)
	}
	if got := Blanks(3); strings.Count(got, "\n") != 2 {
		t.Errorf("Blanks(3) should have 2 newlines, got %q", got)
	}
}

func TestIndentLines(t *testing.T) {
	if got := IndentLines("a\nb", "> "); got != "> a\n> b" {
		t.Errorf("IndentLines = %q", got)
	}
}

func TestWithTitle(t *testing.T) {
	if got := WithTitle("", "body"); got != "body" {
		t.Errorf("empty title should return body unchanged, got %q", got)
	}
	got := WithTitle("Title", "body")
	if !strings.Contains(got, "body") {
		t.Errorf("WithTitle should keep the body, got %q", got)
	}
	if !strings.Contains(got, "\n") {
		t.Errorf("WithTitle should prepend a title bar (extra line), got %q", got)
	}
}

func TestGetOptional(t *testing.T) {
	if got := GetOptional(5); got != 5 {
		t.Errorf("default: got %d", got)
	}
	if got := GetOptional(5, 9); got != 9 {
		t.Errorf("supplied: got %d", got)
	}
	if got := GetOptionalIdx(false, 0, true); got != true {
		t.Errorf("idx 0 supplied: got %v", got)
	}
	if got := GetOptionalIdx("x", 1, "a"); got != "x" {
		t.Errorf("idx out of range should return default, got %q", got)
	}
	if got := GetOptionalIdx("x", 1, "a", "b"); got != "b" {
		t.Errorf("idx 1 supplied: got %q", got)
	}
}

func TestMatchKey(t *testing.T) {
	b := Hint("up", Keys.Up) // carries up/k/w
	if !MatchKey("k", b) {
		t.Error("MatchKey should match a key carried by the binding")
	}
	if MatchKey("z", b) {
		t.Error("MatchKey should reject a key the binding does not carry")
	}
}

func TestPopupBox(t *testing.T) {
	withTitle := PopupBox("Title", "Body", 0)
	if !strings.Contains(withTitle, "Title") || !strings.Contains(withTitle, "Body") {
		t.Errorf("PopupBox should contain title and body, got:\n%s", withTitle)
	}
	if !strings.Contains(withTitle, "│") {
		t.Errorf("PopupBox should draw a border, got:\n%s", withTitle)
	}
	// A title adds a head line + blank, so the titled box is taller than the untitled one.
	noTitle := PopupBox("", "Body", 0)
	if strings.Count(withTitle, "\n") <= strings.Count(noTitle, "\n") {
		t.Errorf("titled popup should be taller than untitled")
	}
}
