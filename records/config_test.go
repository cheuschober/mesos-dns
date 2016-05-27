package records

import (
	"fmt"
	"reflect"
	"testing"
)

func TestNewConfigValidates(t *testing.T) {
	c := NewConfig()
	err := validateIPSources(c.IPSources)
	if err != nil {
		t.Error(err)
	}
	err = validateMasters(c.Masters)
	if err != nil {
		t.Error(err)
	}
	err = validateEnabledServices(c)
	if err == nil {
		t.Error("expected error because no masters and no zk servers are configured by default")
	}
	c.Zk = "foo"
	err = validateEnabledServices(c)
	if err != nil {
		t.Error(err)
	}
}

type testingFile struct {
	file   string
	result map[string]interface{}
	valid  bool
}

func TestReadConfig(t *testing.T) {
	for _, tc := range []testingFile{
		{"/does/not/exist", nil, false},
		{"../factories/scrambled.json", nil, false},
		{"../factories/empty.json", nil, false},
		{"../factories/valid.json", nil, true},
	} {
		c, err := ReadConfig(tc.file)
		if err != nil && tc.valid == true {
			t.Fatal("Error returned: ", err)
		}

		if tc.result != nil {
			for k, v := range tc.result {
				x := reflect.ValueOf(&c).Elem()
				fmt.Printf("%s: %q == %q", k, v, x.FieldByName(k))
			}
		} else {
		}
	}
}
