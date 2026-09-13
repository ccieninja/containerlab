package cisco_iol

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	clabtypes "github.com/srl-labs/containerlab/types"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	return p
}

func newTestIOL(t *testing.T, env map[string]string, startupConfig string) *iol {
	t.Helper()

	n := &iol{}
	cfg := &clabtypes.NodeConfig{
		ShortName:     "r1",
		Env:           env,
		LabDir:        t.TempDir(),
		StartupConfig: startupConfig,
	}

	if err := n.Init(cfg); err != nil {
		t.Fatalf("Init() unexpected error: %v", err)
	}

	return n
}

func TestInitBootstrapConfig(t *testing.T) {
	existing := writeTempFile(t, "base.cfg", "hostname {{ .Hostname }}")

	tests := map[string]struct {
		env      map[string]string
		wantNone bool
		wantFile string
		wantErr  bool
	}{
		"unset": {},
		"none": {
			env:      map[string]string{"CLAB_IOL_BOOTSTRAP_CONFIG": "none"},
			wantNone: true,
		},
		"file": {
			env:      map[string]string{"CLAB_IOL_BOOTSTRAP_CONFIG": existing},
			wantFile: existing,
		},
		"missing file": {
			env:     map[string]string{"CLAB_IOL_BOOTSTRAP_CONFIG": "/nonexistent/base.cfg"},
			wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			n := &iol{}
			cfg := &clabtypes.NodeConfig{Env: tc.env, LabDir: t.TempDir()}

			err := n.Init(cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("Init() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if d := cmp.Diff(tc.wantNone, n.bootstrapNone); d != "" {
				t.Errorf("bootstrapNone mismatch (-want +got):\n%s", d)
			}
			if d := cmp.Diff(tc.wantFile, n.bootstrapCfgFile); d != "" {
				t.Errorf("bootstrapCfgFile mismatch (-want +got):\n%s", d)
			}
		})
	}
}

func TestGenBootConfigBootstrap(t *testing.T) {
	customBase := "hostname {{ .Hostname }}-custom\n{{ .PartialCfg }}"

	tests := map[string]struct {
		env           func(t *testing.T) map[string]string
		startupConfig func(t *testing.T) string
		wantExact     string
		wantSubs      []string
		wantMissing   []string
	}{
		"default baseline": {
			wantSubs: []string{"hostname r1", "interface Ethernet0/0", "vrf definition clab-mgmt"},
		},
		"custom baseline replaces embedded template": {
			env: func(t *testing.T) map[string]string {
				return map[string]string{
					"CLAB_IOL_BOOTSTRAP_CONFIG": writeTempFile(t, "base.cfg", customBase),
				}
			},
			wantSubs:    []string{"hostname r1-custom"},
			wantMissing: []string{"clab-mgmt"},
		},
		"custom baseline layers partial": {
			env: func(t *testing.T) map[string]string {
				return map[string]string{
					"CLAB_IOL_BOOTSTRAP_CONFIG": writeTempFile(t, "base.cfg", customBase),
				}
			},
			startupConfig: func(t *testing.T) string {
				return writeTempFile(t, "extra.partial.cfg", "router ospf 10")
			},
			wantSubs:    []string{"hostname r1-custom", "router ospf 10"},
			wantMissing: []string{"clab-mgmt"},
		},
		"none without startup config": {
			env: func(t *testing.T) map[string]string {
				return map[string]string{"CLAB_IOL_BOOTSTRAP_CONFIG": "none"}
			},
			// CreateFile appends a trailing newline to empty content
			wantExact: "\n",
		},
		"none with partial keeps only the partial": {
			env: func(t *testing.T) map[string]string {
				return map[string]string{"CLAB_IOL_BOOTSTRAP_CONFIG": "none"}
			},
			startupConfig: func(t *testing.T) string {
				return writeTempFile(t, "extra.partial.cfg", "router ospf 10\n")
			},
			wantExact: "router ospf 10\n",
		},
		"full startup config wins over custom baseline": {
			env: func(t *testing.T) map[string]string {
				return map[string]string{
					"CLAB_IOL_BOOTSTRAP_CONFIG": writeTempFile(t, "base.cfg", customBase),
				}
			},
			startupConfig: func(t *testing.T) string {
				return writeTempFile(t, "full.cfg", "hostname {{ .Hostname }}-full\n")
			},
			wantSubs:    []string{"hostname r1-full"},
			wantMissing: []string{"hostname r1-custom"},
		},
		"partial is template rendered": {
			startupConfig: func(t *testing.T) string {
				return writeTempFile(t, "extra.partial.cfg", "hostname {{ .Hostname }}-p\n")
			},
			wantSubs: []string{"hostname r1-p"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var env map[string]string
			if tc.env != nil {
				env = tc.env(t)
			}

			startup := ""
			if tc.startupConfig != nil {
				startup = tc.startupConfig(t)
			}

			n := newTestIOL(t, env, startup)

			if err := n.GenBootConfig(t.Context()); err != nil {
				t.Fatalf("GenBootConfig() unexpected error: %v", err)
			}

			cfg, err := os.ReadFile(path.Join(n.Cfg.LabDir, "boot_config.txt"))
			if err != nil {
				t.Fatal(err)
			}

			if tc.wantExact != "" || (tc.wantSubs == nil && tc.wantMissing == nil) {
				if d := cmp.Diff(tc.wantExact, string(cfg)); d != "" {
					t.Errorf("boot config mismatch (-want +got):\n%s", d)
				}
				return
			}

			for _, sub := range tc.wantSubs {
				if !strings.Contains(string(cfg), sub) {
					t.Errorf("boot config missing %q:\n%s", sub, string(cfg))
				}
			}
			for _, sub := range tc.wantMissing {
				if strings.Contains(string(cfg), sub) {
					t.Errorf("boot config unexpectedly contains %q:\n%s", sub, string(cfg))
				}
			}
		})
	}
}
