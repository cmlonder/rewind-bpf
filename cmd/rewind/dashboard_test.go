package main

import (
	"reflect"
	"testing"
)

func TestDashboardInteractiveShellArgsAvoidHostDotfiles(t *testing.T) {
	tests := []struct {
		name  string
		shell string
		want  []string
	}{
		{name: "zsh", shell: "/bin/zsh", want: []string{"-f", "-i"}},
		{name: "bash", shell: "/bin/bash", want: []string{"--noprofile", "--norc", "-i"}},
		{name: "other", shell: "/bin/fish", want: []string{"-i"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dashboardInteractiveShellArgs(tt.shell); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("args = %#v, want %#v", got, tt.want)
			}
		})
	}
}
