package builtin

import (
	"testing"
)

func TestValidateExternalDNS(t *testing.T) {
	for i, tc := range []validationTest{
		{nil, true},
		{[]string{}, true},
		{[]string{""}, false},
		{[]string{"", ""}, false},
		{[]string{"a"}, false},
		{[]string{"a", "b"}, false},
		{[]string{"1.2.3.4"}, true},
		{[]string{"1.2.3.4.5"}, false},
		{[]string{"1.2.3.4", "1.2.3.4"}, false},
		{[]string{"1.2.3.4", "5.6.7.8"}, true},
		{[]string{"2001:0db8:3c4d:0015:0000:0000:1a2f:1a2b"}, true},
		{[]string{"2001:db8:3c4d:15::1a2f:1a2b"}, true},
		{[]string{"2001:0db8:3c4d:0015:0000:0000:1a2f:1a2b", "2001:db8:3c4d:15::1a2f:1a2b"}, false},
	} {
		validate(t, i+1, tc, validateExternalDNS)
	}
}

type validationTest struct {
	in    []string
	valid bool
}

func validate(t *testing.T, i int, tc validationTest, f func([]string) error) {
	switch err := f(tc.in); {
	case (err == nil && tc.valid) || (err != nil && !tc.valid):
		return // valid
	case tc.valid:
		t.Fatalf("test %d failed, unexpected error validating remote DNS server %v: %v", i, tc.in, err)
	default:
		t.Fatalf("test %d failed, expected validation error for ExternalDNS(%d) %v", i, len(tc.in), tc.in)
	}
}
