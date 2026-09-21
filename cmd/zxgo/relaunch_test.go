package main

import (
	"strings"
	"testing"
)

// The relaunched process gets the command line the first one was given, with the machine
// flags removed and the model re-added from the settings. Both spellings -model can arrive in
// are handled, because writing "-model pentagon" while "-model 48k" is also on the line would
// leave the two of them to fight over which wins - and the flag the user gave is exactly what
// a relaunch has to displace, since a per-run flag beats the file the window wrote.
func TestRelaunchArgsReplaceTheModel(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "no model on the line",
			args: []string{"-tap", "game.tap"},
			want: []string{"-tap", "game.tap", "-model", "pentagon"},
		},
		{
			name: "the flag was named in two words",
			args: []string{"-model", "48k", "-tap", "game.tap"},
			want: []string{"-tap", "game.tap", "-model", "pentagon"},
		},
		{
			name: "the flag was named with an equals sign",
			args: []string{"-model=48k", "-disk", "game.trd"},
			want: []string{"-disk", "game.trd", "-model", "pentagon"},
		},
		{
			// A value that looks like a flag is still the model flag's value, and must not
			// be left behind as an argument of its own.
			name: "the model's value is not mistaken for an argument",
			args: []string{"-model", "-weird"},
			want: []string{"-model", "pentagon"},
		},
		{
			// The sound flags go too: they beat the file at startup, so leaving one on the
			// line would undo the change the user just made in the window.
			name: "the sound flags are dropped",
			args: []string{"-turbosound", "-gs", "-snow", "-tap", "game.tap"},
			want: []string{"-tap", "game.tap", "-model", "pentagon"},
		},
		{
			name: "a sound flag written with an equals sign is dropped too",
			args: []string{"-turbosoundfm=false", "-gs=true", "-log", "info"},
			want: []string{"-log", "info", "-model", "pentagon"},
		},
		{
			// A flag this list does not name is the user's and stays: the relaunch is the
			// same run on another machine, not a fresh start.
			name: "everything else is kept",
			args: []string{"-fast-tape", "-no-fdc-timing", "-log", "debug"},
			want: []string{"-fast-tape", "-no-fdc-timing", "-log", "debug", "-model", "pentagon"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := relaunchArgs(c.args, "pentagon")
			if strings.Join(got, " ") != strings.Join(c.want, " ") {
				t.Errorf("relaunchArgs(%v) = %v; want %v", c.args, got, c.want)
			}
		})
	}

	// The media, the replay and the session paths are carried across: a relaunch keeps the
	// tape and the disk the machine was given.
	in := []string{"-tap", "game.tap", "-disk", "game.trd", "-replay", "r.replay", "-session", "/tmp/s"}
	got := relaunchArgs(in, "128k")
	for _, want := range in {
		found := false
		for _, a := range got {
			if a == want {
				found = true
			}
		}
		if !found {
			t.Errorf("relaunchArgs dropped %q: %v", want, got)
		}
	}

	// A model the settings do not name leaves the command line alone rather than writing an
	// empty model, which would name no machine at all.
	if got := relaunchArgs([]string{"-tap", "t.tap"}, ""); len(got) != 2 || got[1] != "t.tap" {
		t.Errorf("relaunchArgs with no model = %v; want the line unchanged", got)
	}
}
