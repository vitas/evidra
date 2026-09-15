package evidence

// JCS canonicalization (RFC 8785) for the v2 evidence model.
//
// Fingerprints have to survive a key reorder, a whitespace change, and a
// re-serialization by an intermediary, or "same arguments" means nothing. That
// is why arguments are canonicalized before HMAC rather than hashed as received.
// RFC 8785 is chosen because it is specified in terms ordinary JSON
// implementations can satisfy exactly: sort object keys by UTF-16 code unit,
// write numbers in ECMAScript's shortest form, use only the escapes JSON.stringify
// uses. It is implemented here rather than taken from a dependency because the
// Core build stays stdlib-only.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// CanonicalizeJCS returns the RFC 8785 canonical JSON encoding of v.
//
// The value is re-encoded and re-decoded first so that callers may pass any
// JSON-compatible Go value: structs, maps with non-string keys are rejected by
// encoding/json, and the decoded form is exactly the six JSON types the
// algorithm is defined over. Numbers are decoded as json.Number and converted to
// float64, which is what the spec's "IEEE 754 double" requirement means.
func CanonicalizeJCS(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("canonicalize: encode input: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("canonicalize: decode input: %w", err)
	}
	if dec.More() {
		return nil, errors.New("canonicalize: input is not a single JSON value")
	}
	var out strings.Builder
	if err := writeJCS(&out, decoded); err != nil {
		return nil, err
	}
	return []byte(out.String()), nil
}

func writeJCS(out *strings.Builder, v any) error {
	switch t := v.(type) {
	case nil:
		out.WriteString("null")
	case bool:
		if t {
			out.WriteString("true")
		} else {
			out.WriteString("false")
		}
	case string:
		writeJCSString(out, t)
	case json.Number:
		f, err := strconv.ParseFloat(t.String(), 64)
		if err != nil {
			return fmt.Errorf("canonicalize: number %s: %w", t, err)
		}
		return writeJCSNumber(out, f)
	case []any:
		out.WriteByte('[')
		for i, el := range t {
			if i > 0 {
				out.WriteByte(',')
			}
			if err := writeJCS(out, el); err != nil {
				return err
			}
		}
		out.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return utf16Less(keys[i], keys[j]) })
		out.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				out.WriteByte(',')
			}
			writeJCSString(out, k)
			out.WriteByte(':')
			if err := writeJCS(out, t[k]); err != nil {
				return err
			}
		}
		out.WriteByte('}')
	default:
		return fmt.Errorf("canonicalize: unsupported JSON value %T", v)
	}
	return nil
}

// writeJCSString escapes exactly the characters JSON.stringify escapes. Go's own
// escaper uses lowercase hex and escapes more characters, which would produce a
// different digest for the same document, so the set is spelled out.
func writeJCSString(out *strings.Builder, s string) {
	out.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			out.WriteString(`\"`)
		case '\\':
			out.WriteString(`\\`)
		case '\b':
			out.WriteString(`\b`)
		case '\f':
			out.WriteString(`\f`)
		case '\n':
			out.WriteString(`\n`)
		case '\r':
			out.WriteString(`\r`)
		case '\t':
			out.WriteString(`\t`)
		default:
			// Invalid UTF-8 arrives here as U+FFFD from the rune range, which is
			// also RFC 8785's replacement rule for lone surrogates.
			if r < 0x20 {
				fmt.Fprintf(out, `\u%04X`, r)
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
}

// writeJCSNumber implements ECMAScript NumberToString for finite doubles.
func writeJCSNumber(out *strings.Builder, f float64) error {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return errors.New("canonicalize: JSON has no NaN or infinity")
	}
	if f == 0 {
		out.WriteString("0")
		return nil
	}
	if f < 0 {
		out.WriteByte('-')
		f = -f
	}
	// Go's shortest 'e' form gives the significant digits and the decimal
	// exponent; RFC 8785 needs those re-split into the spec's k and n.
	sci := strconv.FormatFloat(f, 'e', -1, 64)
	mantissa, expText, _ := strings.Cut(sci, "e")
	digits := strings.ReplaceAll(mantissa, ".", "")
	exp, err := strconv.Atoi(expText)
	if err != nil {
		return fmt.Errorf("canonicalize: exponent %q: %w", expText, err)
	}
	k := len(digits)
	n := exp + 1 // digit-count position of the decimal point
	switch {
	case k <= n && n <= 21:
		out.WriteString(digits)
		out.WriteString(strings.Repeat("0", n-k))
	case 0 < n && n <= 21:
		out.WriteString(digits[:n])
		out.WriteByte('.')
		out.WriteString(digits[n:])
	case -6 < n && n <= 0:
		out.WriteString("0.")
		out.WriteString(strings.Repeat("0", -n))
		out.WriteString(digits)
	case k == 1:
		out.WriteString(digits)
		writeJCSExponent(out, n-1)
	default:
		out.WriteString(digits[:1])
		out.WriteByte('.')
		out.WriteString(digits[1:])
		writeJCSExponent(out, n-1)
	}
	return nil
}

func writeJCSExponent(out *strings.Builder, e int) {
	if e >= 0 {
		fmt.Fprintf(out, "e+%d", e)
		return
	}
	fmt.Fprintf(out, "e-%d", -e)
}

// utf16Less orders strings by UTF-16 code unit, which is what RFC 8785 requires
// and what byte ordering of UTF-8 is not: a supplementary-plane character sorts
// before U+E000..U+FFFF in code-unit order but after them in code-point order.
func utf16Less(a, b string) bool {
	ua, ub := utf16.Encode([]rune(a)), utf16.Encode([]rune(b))
	for i := 0; i < len(ua) && i < len(ub); i++ {
		if ua[i] != ub[i] {
			return ua[i] < ub[i]
		}
	}
	return len(ua) < len(ub)
}
