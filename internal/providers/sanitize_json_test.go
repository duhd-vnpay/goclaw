package providers

import (
	"bytes"
	"testing"
)

func TestSanitizeJSONSurrogates_NoSurrogates(t *testing.T) {
	input := []byte(`{"role":"user","content":"Hello world"}`)
	result := sanitizeJSONSurrogates(input)
	if !bytes.Equal(result, input) {
		t.Errorf("expected unchanged output, got %s", result)
	}
}

func TestSanitizeJSONSurrogates_HighSurrogate(t *testing.T) {
	input := []byte(`{"content":"before\uD800after"}`)
	result := sanitizeJSONSurrogates(input)
	expected := []byte(`{"content":"before\uFFFDafter"}`)
	if !bytes.Equal(result, expected) {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestSanitizeJSONSurrogates_LowSurrogate(t *testing.T) {
	input := []byte(`{"content":"before\uDC90after"}`)
	result := sanitizeJSONSurrogates(input)
	expected := []byte(`{"content":"before\uFFFDafter"}`)
	if !bytes.Equal(result, expected) {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestSanitizeJSONSurrogates_MultipleSurrogates(t *testing.T) {
	input := []byte(`{"text":"\uD800 and \uDBFF and \uDC00 and \uDFFF"}`)
	result := sanitizeJSONSurrogates(input)
	expected := []byte(`{"text":"\uFFFD and \uFFFD and \uFFFD and \uFFFD"}`)
	if !bytes.Equal(result, expected) {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestSanitizeJSONSurrogates_SurrogatePair(t *testing.T) {
	// Surrogate pairs (\uD800\uDC00) are also replaced — Go's json.Marshal
	// never produces them (it emits UTF-8 directly), so if they appear in
	// json.RawMessage they indicate corruption.
	input := []byte(`{"text":"\uD83D\uDE00"}`)
	result := sanitizeJSONSurrogates(input)
	expected := []byte(`{"text":"\uFFFD\uFFFD"}`)
	if !bytes.Equal(result, expected) {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestSanitizeJSONSurrogates_CaseInsensitive(t *testing.T) {
	input := []byte(`{"text":"\uDaB3\udcff"}`)
	result := sanitizeJSONSurrogates(input)
	expected := []byte(`{"text":"\uFFFD\uFFFD"}`)
	if !bytes.Equal(result, expected) {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestSanitizeJSONSurrogates_ValidUnicodeEscapesUntouched(t *testing.T) {
	// Non-surrogate unicode escapes should not be modified
	input := []byte(`{"text":"\u0041\u00E9\u4E16\uFFFD"}`)
	result := sanitizeJSONSurrogates(input)
	if !bytes.Equal(result, input) {
		t.Errorf("expected unchanged output, got %s", result)
	}
}

func TestSanitizeJSONSurrogates_VietnameseUTF8Untouched(t *testing.T) {
	// Real Vietnamese text encoded as UTF-8 bytes (not \u escapes) must pass through
	input := []byte(`{"content":"Đọc Gmail omc@vnpay.vn trong 12 giờ qua"}`)
	result := sanitizeJSONSurrogates(input)
	if !bytes.Equal(result, input) {
		t.Errorf("expected unchanged output for Vietnamese UTF-8")
	}
}

func TestSanitizeJSONSurrogates_LargePayload(t *testing.T) {
	// Simulate a large request body (~64KB) with a surrogate buried inside
	prefix := bytes.Repeat([]byte("A"), 61840)
	input := append(prefix, []byte(`\uD800rest`)...)
	result := sanitizeJSONSurrogates(input)
	expected := append(prefix, []byte(`\uFFFDrest`)...)
	if !bytes.Equal(result, expected) {
		t.Error("expected surrogate at position ~61840 to be replaced")
	}
}
