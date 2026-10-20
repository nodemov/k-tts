package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestGroupFiles(t *testing.T) {
	var files []string
	for _, name := range []string{"0081", "0082", "0083", "0084", "0085"} {
		files = append(files, filepath.Join("output", name+".mp3"))
	}

	groups := groupFiles(files, 2)
	var names []string
	for _, g := range groups {
		names = append(names, g.name)
	}
	want := []string{"0081-0082", "0083-0084", "0085"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	if len(groups[0].files) != 2 || len(groups[2].files) != 1 {
		t.Fatalf("unexpected group sizes: %v", groups)
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		args    []string
		want    combineOptions
		wantErr bool
	}{
		{args: nil, want: combineOptions{size: 10, inputDir: "output", outputDir: filepath.Join("output", "combined")}},
		{args: []string{"5"}, want: combineOptions{size: 5, inputDir: "output", outputDir: filepath.Join("output", "combined")}},
		{args: []string{"5", "in"}, want: combineOptions{size: 5, inputDir: "in", outputDir: filepath.Join("in", "combined")}},
		{args: []string{"5", "in", "out"}, want: combineOptions{size: 5, inputDir: "in", outputDir: "out"}},
		{args: []string{"0"}, wantErr: true},
		{args: []string{"abc"}, wantErr: true},
		{args: []string{"1", "2", "3", "4"}, wantErr: true},
	}
	for _, tt := range tests {
		got, err := parseArgs(tt.args)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseArgs(%v) expected error", tt.args)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseArgs(%v) error: %v", tt.args, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseArgs(%v) = %+v, want %+v", tt.args, got, tt.want)
		}
	}
}
