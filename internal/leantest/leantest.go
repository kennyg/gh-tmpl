// Package leantest reads the differential test cases that the Lean models
// in verify/ write. Each case is one JSON object on one line.
//
// Run "mise run verify" to check the proofs and write the cases again.
package leantest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Each decodes every case in testdata/lean/name and calls fn with it. A
// case that fails to decode fails the test.
func Each[T any](t *testing.T, name string, fn func(t *testing.T, line int, c T)) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "lean", name))
	if err != nil {
		t.Fatalf("%v (run \"mise run verify\" to write the cases)", err)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	n := 0
	for sc.Scan() {
		n++
		var c T
		dec := json.NewDecoder(bytes.NewReader(sc.Bytes()))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			t.Fatalf("%s:%d: %v", name, n, err)
		}
		fn(t, n, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatalf("%s has no cases", name)
	}
}
