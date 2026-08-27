package cisco_iol

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	clabtypes "github.com/srl-labs/containerlab/types"
)

func TestInitPid(t *testing.T) {
	tests := map[string]struct {
		index         int
		env           map[string]string
		wantPid       string
		wantNvramFile string
		wantErr       bool
	}{
		"default": {
			index:         2,
			wantPid:       "3",
			wantNvramFile: "nvram_00003",
		},
		"offset": {
			index:         0,
			env:           map[string]string{"CLAB_IOL_PID_OFFSET": "64"},
			wantPid:       "65",
			wantNvramFile: "nvram_00065",
		},
		"invalid offset": {
			env:     map[string]string{"CLAB_IOL_PID_OFFSET": "abc"},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			n := &iol{}
			cfg := &clabtypes.NodeConfig{Index: tc.index, Env: tc.env, LabDir: t.TempDir()}

			err := n.Init(cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Init() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if d := cmp.Diff(tc.wantPid, n.Pid); d != "" {
				t.Errorf("Pid mismatch (-want +got):\n%s", d)
			}
			if d := cmp.Diff(tc.wantNvramFile, n.nvramFile); d != "" {
				t.Errorf("nvramFile mismatch (-want +got):\n%s", d)
			}
			if d := cmp.Diff(tc.wantPid, n.Cfg.Env["IOL_PID"]); d != "" {
				t.Errorf("IOL_PID env mismatch (-want +got):\n%s", d)
			}
		})
	}
}
