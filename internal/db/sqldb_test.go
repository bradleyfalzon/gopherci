package db

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestTrim(t *testing.T) {
	b := []byte("Go is a general-purpose language designed with systems programming in mind.")

	tests := []struct {
		max  int
		want []byte
	}{
		{len(b) + 100, []byte("Go is a general-purpose language designed with systems programming in mind.")},
		{len(b), []byte("Go is a general-purpose language designed with systems programming in mind.")},
		{10, []byte("Go is...65 bytes suppressed...mind.")},
	}

	for _, test := range tests {
		have := trim(b, test.max)
		if diff := cmp.Diff(string(have), string(test.want)); diff != "" {
			t.Errorf("not equal (-have +want)\n%s", diff)
		}
	}
}
