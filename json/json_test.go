package json

import (
	"testing"
)

type user struct {
	Blog string `json:"blog"`
	Name string `json:"name"`
}

func TestMarshal(t *testing.T) {
	u := user{"xwi88", "https://github.com/xwi88"}
	t.Logf("pkg name: %v", PKG)

	b, err := Marshal(u)
	if err != nil {
		t.Error(err)
	} else {
		t.Logf("\ndata: %#v, \nMarshal: %v", u, string(b))
	}

	var uu user
	if err := Unmarshal(b, &uu); err != nil {
		t.Error(err)
	} else {
		t.Logf("\ndata: %#v, \nUnmarshal: %#v", u, uu)
	}
}

func TestMarshalIndent(t *testing.T) {
	u := user{"xwi88", "https://github.com/xwi88"}
	t.Logf("pkg name: %v", PKG)

	b, err := MarshalIndent(u, "", "  ")
	if err != nil {
		t.Error(err)
	} else {
		t.Logf("\ndata: %#v, \nMarshalIndent: %v", u, string(b))
	}

	var uu user
	if err := Unmarshal(b, &uu); err != nil {
		t.Error(err)
	} else {
		t.Logf("\ndata: %#v, \nUnmarshal: %#v", u, uu)
	}
}

func TestValid(t *testing.T) {
	u := user{"xwi88", "https://github.com/xwi88"}
	t.Logf("pkg name: %v", PKG)

	b, err := Marshal(u)
	if err != nil {
		t.Error(err)
	} else {
		t.Logf("\ndata: %#v, \nMarshal: %v", u, string(b))
	}

	if Valid(b) {
		t.Logf("\ndata: %v, is valid json", string(b))
	} else {
		t.Errorf("\ndata: %v, is not valid json", string(b))
	}

	bb := append([]byte{96}, b...)
	bb = append(bb, 96)

	if Valid(bb) {
		t.Errorf("\ndata: %v, is not valid json", string(bb))
	} else {
		t.Logf("\ndata: %v, is not valid json", string(bb))
	}
}
