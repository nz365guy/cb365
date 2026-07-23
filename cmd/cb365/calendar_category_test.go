package main

import (
	"reflect"
	"testing"
)

func TestNormaliseCategoriesFoldsDuplicatesPreservingOrder(t *testing.T) {
	got, err := normaliseCategories([]string{"Client", "Focus", "Client", " Focus ", "Admin"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Client", "Focus", "Admin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNormaliseCategoriesRejectsEmptyValues(t *testing.T) {
	for _, values := range [][]string{{""}, {"Client", "  "}, {"\t"}} {
		if _, err := normaliseCategories(values); err == nil {
			t.Fatalf("expected error for %q", values)
		}
	}
}

func TestNormaliseCategoriesTrimsWhitespace(t *testing.T) {
	got, err := normaliseCategories([]string{"  Deep Work  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0] != "Deep Work" {
		t.Fatalf("got %v, want [Deep Work]", got)
	}
}
