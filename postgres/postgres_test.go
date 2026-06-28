package postgres

import "testing"

func Test_New_MissingHost(t *testing.T) {
	_, err := New(t.Context(), Options{})
	if err == nil {
		t.Fatal("expected error for empty host")
	}
}

func Test_New_MissingDB(t *testing.T) {
	_, err := New(t.Context(), Options{Host: "localhost"})
	if err == nil {
		t.Fatal("expected error for empty db name")
	}
}
