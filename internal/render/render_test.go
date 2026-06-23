package render

import (
	"strings"
	"testing"
)

// TestJSONPreservesLargeIntegers is the verbatim-passthrough guard: a 64-bit integer (e.g. a Gmail
// thread/message id) inside the server body must survive -o json digit-for-digit. Decoding into `any`
// would coerce it to float64 and lose precision past 2^53, so json.Indent must operate on the raw bytes.
func TestJSONPreservesLargeIntegers(t *testing.T) {
	const gmailID = "1866870901077892614" // a real Gmail 64-bit id; > 2^53
	raw := []byte(`{"thread_id":` + gmailID + `,"nested":{"msg":12345678901234567890}}`)

	var out strings.Builder
	if err := JSON(&out, raw); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, gmailID) {
		t.Errorf("64-bit id mangled: %q not found in:\n%s", gmailID, got)
	}
	if !strings.Contains(got, "12345678901234567890") {
		t.Errorf("nested 64-bit id mangled in:\n%s", got)
	}
	if strings.Contains(got, "1866870901077892600") {
		t.Errorf("id lost precision (float64 round-trip) in:\n%s", got)
	}
}

// TestJSONReindentsVerbatim confirms re-indentation is the only transform: keys, values, and escaping are
// reproduced exactly, just pretty-printed.
func TestJSONReindentsVerbatim(t *testing.T) {
	raw := []byte(`{"a":1,"b":"x"}`)
	var out strings.Builder
	if err := JSON(&out, raw); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	want := "{\n  \"a\": 1,\n  \"b\": \"x\"\n}\n"
	if out.String() != want {
		t.Errorf("re-indent mismatch:\n got %q\nwant %q", out.String(), want)
	}
}

// TestJSONNonJSONPassthrough: a non-JSON body (a plain-text proxy error) is emitted as-is, not mangled.
func TestJSONNonJSONPassthrough(t *testing.T) {
	var out strings.Builder
	if err := JSON(&out, []byte("Bad Gateway")); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if out.String() != "Bad Gateway\n" {
		t.Errorf("non-JSON passthrough = %q", out.String())
	}
}
