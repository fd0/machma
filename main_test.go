package main

import (
	"bufio"
	"reflect"
	"strings"
	"testing"
)

var nullTests = []struct {
	input  string
	output []string
}{
	{
		"foo\x00bar\x00baz",
		[]string{"foo", "bar", "baz"},
	},
	{
		"foo\x00",
		[]string{"foo"},
	},
}

func TestScanNullSeparatedValues(t *testing.T) {
	t.Parallel()

	for i, test := range nullTests {
		sc := bufio.NewScanner(strings.NewReader(test.input))
		sc.Split(ScanNullSeparatedValues)

		var output []string
		for sc.Scan() {
			output = append(output, sc.Text())
		}

		if !reflect.DeepEqual(output, test.output) {
			t.Errorf("test %d failed: want %v, got %v", i, test.output, output)
		}
	}
}
