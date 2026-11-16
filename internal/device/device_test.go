package device

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	in := Input{Name: "  NAS ", MAC: "AA-BB-CC-DD-EE-FF", IP: " 192.168.1.5", Broadcast: "192.168.1.255", Description: " hi "}
	if err := in.Normalize(9); err != nil {
		t.Fatal(err)
	}
	if in.Name != "NAS" || in.MAC != "aa:bb:cc:dd:ee:ff" || in.IP != "192.168.1.5" || in.Port != 9 || in.Description != "hi" {
		t.Errorf("not normalised: %+v", in)
	}
}

func TestNormalizeErrors(t *testing.T) {
	good := Input{Name: "x", MAC: "aa:bb:cc:dd:ee:ff"}
	cases := map[string]func(*Input){
		"name":        func(i *Input) { i.Name = "  " },
		"mac":         func(i *Input) { i.MAC = "zz" },
		"ip":          func(i *Input) { i.IP = "999.1.1.1" },
		"broadcast":   func(i *Input) { i.Broadcast = "::1" },
		"port":        func(i *Input) { i.Port = 70000 },
		"description": func(i *Input) { i.Description = strings.Repeat("a", 501) },
	}
	for field, mutate := range cases {
		in := good
		mutate(&in)
		err := in.Normalize(9)
		var v ValidationError
		if !errors.As(err, &v) || v[field] == "" {
			t.Errorf("%s: err = %v", field, err)
		}
	}
	neg := good
	neg.Port = -1
	if err := neg.Normalize(9); err == nil {
		t.Error("negative port accepted")
	}
	v6 := good
	v6.IP = "::ffff:1.2.3.4"
	if err := v6.Normalize(9); err == nil {
		t.Error("IPv6-form address accepted")
	}
}
