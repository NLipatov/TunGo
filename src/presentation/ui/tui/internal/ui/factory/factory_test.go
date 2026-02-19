package uifactory

import "testing"

func TestNewDefaultBundle_ReturnsNonNilFields(t *testing.T) {
	bundle := NewDefaultBundle()
	if bundle.SelectorFactory == nil {
		t.Fatal("expected non-nil SelectorFactory")
	}
	if bundle.TextInputFactory == nil {
		t.Fatal("expected non-nil TextInputFactory")
	}
	if bundle.TextAreaFactory == nil {
		t.Fatal("expected non-nil TextAreaFactory")
	}
}
