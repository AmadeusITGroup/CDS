package command

import "testing"

func TestSpcInitRejectsProfile(t *testing.T) {
	sci := &spcInit{}
	sci.command()
	if err := sci.cmd.Flags().Set("use-profile", "nosuch"); err != nil {
		t.Fatal(err)
	}
	if err := sci.runE(sci.cmd, nil); err == nil {
		t.Fatal("expected error for --use-profile")
	}
	sci.profile = ""
	if err := sci.runE(sci.cmd, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
