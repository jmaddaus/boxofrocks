package cli

import (
	"reflect"
	"testing"
)

func TestReorderArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "flags already first",
			in:   []string{"-p", "1", "-t", "bug", "Fix login"},
			want: []string{"-p", "1", "-t", "bug", "Fix login"},
		},
		{
			name: "positional before flags",
			in:   []string{"Fix login", "-p", "1", "-t", "bug"},
			want: []string{"-p", "1", "-t", "bug", "Fix login"},
		},
		{
			name: "positional between flags",
			in:   []string{"-p", "1", "Fix login", "-t", "bug"},
			want: []string{"-p", "1", "-t", "bug", "Fix login"},
		},
		{
			name: "equals syntax",
			in:   []string{"Fix login", "-p=1", "--type=bug"},
			want: []string{"-p=1", "--type=bug", "Fix login"},
		},
		{
			name: "no flags",
			in:   []string{"Fix login"},
			want: []string{"Fix login"},
		},
		{
			name: "no positional",
			in:   []string{"-p", "1"},
			want: []string{"-p", "1"},
		},
		{
			name: "empty",
			in:   []string{},
			want: nil,
		},
		{
			name: "update style with id first",
			in:   []string{"5", "--status", "in_progress", "--comment", "started"},
			want: []string{"--status", "in_progress", "--comment", "started", "5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := reorderArgs(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("reorderArgs(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
