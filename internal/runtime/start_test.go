package runtime

import (
	"slices"
	"testing"
)

func TestStart(t *testing.T) {
	lines, stop, err := Start([]string{"sh", "-c", "echo out; echo err >&2"})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	var got []string
	for l := range lines {
		got = append(got, l)
	}
	slices.Sort(got)
	if !slices.Equal(got, []string{"err", "out"}) {
		t.Fatal(got)
	}
	if _, _, err := Start([]string{"no-such-binary-xyz"}); err == nil {
		t.Fatal("expected error")
	}
}
