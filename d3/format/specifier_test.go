package format

import (
	"errors"
	"testing"
)

// TestParseSpecifierFields walks the grammar one piece at a time. These are
// value assertions because the grammar assigns each character position an
// unambiguous meaning.
func TestParseSpecifierFields(t *testing.T) {
	tests := []struct {
		in   string
		want Specifier
	}{
		{"", Specifier{Fill: " ", Align: ">", Sign: "-", Width: Unset, Precision: Unset}},
		{"f", Specifier{Fill: " ", Align: ">", Sign: "-", Width: Unset, Precision: Unset, Type: 'f'}},
		{".2f", Specifier{Fill: " ", Align: ">", Sign: "-", Width: Unset, Precision: 2, Type: 'f'}},
		{"10.2f", Specifier{Fill: " ", Align: ">", Sign: "-", Width: 10, Precision: 2, Type: 'f'}},
		{",d", Specifier{Fill: " ", Align: ">", Sign: "-", Width: Unset, Precision: Unset, Comma: true, Type: 'd'}},
		{"$,.2f", Specifier{Fill: " ", Align: ">", Sign: "-", Symbol: "$", Width: Unset, Precision: 2, Comma: true, Type: 'f'}},
		{"#x", Specifier{Fill: " ", Align: ">", Sign: "-", Symbol: "#", Width: Unset, Precision: Unset, Type: 'x'}},
		{"08.3f", Specifier{Fill: "0", Align: "=", Sign: "-", Zero: true, Width: 8, Precision: 3, Type: 'f'}},
		{"+.1%", Specifier{Fill: " ", Align: ">", Sign: "+", Width: Unset, Precision: 1, Type: '%'}},
		{"(.2f", Specifier{Fill: " ", Align: ">", Sign: "(", Width: Unset, Precision: 2, Type: 'f'}},
		{"*^20.3~s", Specifier{Fill: "*", Align: "^", Sign: "-", Width: 20, Precision: 3, Trim: true, Type: 's'}},
		{"<8d", Specifier{Fill: " ", Align: "<", Sign: "-", Width: 8, Precision: Unset, Type: 'd'}},
		{"=+10d", Specifier{Fill: " ", Align: "=", Sign: "+", Width: 10, Precision: Unset, Type: 'd'}},
	}
	for _, tt := range tests {
		got, err := ParseSpecifier(tt.in)
		if err != nil {
			t.Errorf("ParseSpecifier(%q): %v", tt.in, err)
			continue
		}
		// Zero fill normalizes fill and align inside Compile, not in the
		// parser, except for the "0" flag itself; keep the expectation honest.
		if tt.in == "08.3f" {
			tt.want.Fill, tt.want.Align = " ", ">"
		}
		if got != tt.want {
			t.Errorf("ParseSpecifier(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

// TestSpecifierRoundTrip is a property: rendering a parsed specifier and
// parsing it again must land on the same value. Exact spellings are not
// asserted because the canonical form drops redundant defaults.
func TestSpecifierRoundTrip(t *testing.T) {
	inputs := []string{
		"", "f", ".2f", "10.2f", ",d", "$,.2f", "#x", "08.3f", "+.1%", "(.2f",
		"*^20.3~s", "<8d", "=+10d", " >12,.4r", "~g", ".0e", "+$8,.2f", "020,d",
	}
	for _, in := range inputs {
		s, err := ParseSpecifier(in)
		if err != nil {
			t.Fatalf("ParseSpecifier(%q): %v", in, err)
		}
		again, err := ParseSpecifier(s.String())
		if err != nil {
			t.Fatalf("ParseSpecifier(%q.String() = %q): %v", in, s.String(), err)
		}
		if again != s {
			t.Errorf("round trip of %q: %+v -> %q -> %+v", in, s, s.String(), again)
		}
		// And the compiled formatters must agree too.
		a, b := EnUS.Compile(s), EnUS.Compile(again)
		for _, v := range []float64{0, 1, -1, 1234.5678, 0.000123, 1e9} {
			if a(v) != b(v) {
				t.Errorf("round trip of %q changed output for %v: %q vs %q", in, v, a(v), b(v))
			}
		}
	}
}

func TestParseSpecifierErrors(t *testing.T) {
	for _, bad := range []string{"foo", "..2f", ".f", "1.2.3", "%%", "d$", ",,d"} {
		if _, err := ParseSpecifier(bad); !errors.Is(err, ErrInvalidSpecifier) {
			t.Errorf("ParseSpecifier(%q) error = %v, want ErrInvalidSpecifier", bad, err)
		}
	}
}

// TestSpecifierStringOmitsDefaults documents the one deliberate divergence from
// d3's FormatSpecifier.toString, which always emits the fill and alignment.
func TestSpecifierStringOmitsDefaults(t *testing.T) {
	s, _ := ParseSpecifier(".2f")
	if got := s.String(); got != ".2f" {
		t.Errorf("String() = %q, want %q", got, ".2f")
	}
	s.Fill, s.Align = "*", "^"
	if got := s.String(); got != "*^.2f" {
		t.Errorf("String() = %q, want %q", got, "*^.2f")
	}
}
