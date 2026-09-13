package cisco_iol

import (
	"os"
	"path"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	clablinks "github.com/srl-labs/containerlab/links"
	clabtypes "github.com/srl-labs/containerlab/types"
)

func newTestIOL(t *testing.T, env map[string]string) *iol {
	t.Helper()

	n := &iol{}
	cfg := &clabtypes.NodeConfig{Env: env, LabDir: t.TempDir()}

	if err := n.Init(cfg); err != nil {
		t.Fatalf("Init() unexpected error: %v", err)
	}

	return n
}

func vethEndpoint(name string) *clablinks.EndpointVeth {
	return &clablinks.EndpointVeth{
		EndpointGeneric: clablinks.EndpointGeneric{IfaceName: name},
	}
}

func TestInitMgmtIntf(t *testing.T) {
	tests := map[string]struct {
		env          map[string]string
		wantMgmtIntf string
		wantMgmtIdx  int
		wantErr      bool
	}{
		"default": {
			wantMgmtIntf: "Ethernet0/0",
			wantMgmtIdx:  0,
		},
		"explicit default": {
			env:          map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet0/0"},
			wantMgmtIntf: "Ethernet0/0",
			wantMgmtIdx:  0,
		},
		"relocated": {
			env:          map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"},
			wantMgmtIntf: "Ethernet3/3",
			wantMgmtIdx:  15,
		},
		"short form": {
			env:          map[string]string{"CLAB_IOL_MGMT_INTF": "e1/2"},
			wantMgmtIntf: "Ethernet1/2",
			wantMgmtIdx:  6,
		},
		"invalid name": {
			env:     map[string]string{"CLAB_IOL_MGMT_INTF": "mgmt0"},
			wantErr: true,
		},
		"port out of range": {
			env:     map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/9"},
			wantErr: true,
		},
		"slot out of range": {
			env:     map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet12/0"},
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

			if d := cmp.Diff(tc.wantMgmtIntf, n.mgmtIntf); d != "" {
				t.Errorf("mgmtIntf mismatch (-want +got):\n%s", d)
			}
			if d := cmp.Diff(tc.wantMgmtIdx, n.mgmtLinuxIdx); d != "" {
				t.Errorf("mgmtLinuxIdx mismatch (-want +got):\n%s", d)
			}
		})
	}
}

func TestGetMappedInterfaceName(t *testing.T) {
	tests := map[string]struct {
		env     map[string]string
		ifName  string
		want    string
		wantErr bool
	}{
		"default e0/1":            {ifName: "e0/1", want: "eth1"},
		"default Ethernet1/0":     {ifName: "Ethernet1/0", want: "eth4"},
		"default e0/0 to eth0":    {ifName: "e0/0", want: "eth0"},
		"relocated e0/0 swaps":    {env: map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"}, ifName: "Ethernet0/0", want: "eth15"},
		"relocated mgmt to eth0":  {env: map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"}, ifName: "Ethernet3/3", want: "eth0"},
		"relocated others stable": {env: map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"}, ifName: "Ethernet0/1", want: "eth1"},
		"no slot/port":            {ifName: "foo", wantErr: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			n := newTestIOL(t, tc.env)

			got, err := n.GetMappedInterfaceName(tc.ifName)
			if (err != nil) != tc.wantErr {
				t.Fatalf("GetMappedInterfaceName() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}

			if d := cmp.Diff(tc.want, got); d != "" {
				t.Errorf("mapped name mismatch (-want +got):\n%s", d)
			}
		})
	}
}

func TestGenInterfaceConfigDefault(t *testing.T) {
	n := newTestIOL(t, nil)
	n.Endpoints = append(n.Endpoints, vethEndpoint("eth1"), vethEndpoint("eth2"))

	if err := n.GenInterfaceConfig(t.Context()); err != nil {
		t.Fatalf("GenInterfaceConfig() unexpected error: %v", err)
	}

	netmap, err := os.ReadFile(path.Join(n.Cfg.LabDir, "NETMAP"))
	if err != nil {
		t.Fatal(err)
	}
	wantNetmap := "1:0/0 513:0/0\n1:0/1 513:0/1\n1:0/2 513:0/2\n"
	if d := cmp.Diff(wantNetmap, string(netmap)); d != "" {
		t.Errorf("NETMAP mismatch (-want +got):\n%s", d)
	}

	iouyap, err := os.ReadFile(path.Join(n.Cfg.LabDir, "iouyap.ini"))
	if err != nil {
		t.Fatal(err)
	}
	wantIouyap := "[default]\nbase_port = 49000\nnetmap = /iol/NETMAP\n" +
		"[513:0/0]\neth_dev = eth0\n" +
		"[513:0/1]\neth_dev = eth1\n" +
		"[513:0/2]\neth_dev = eth2\n"
	if d := cmp.Diff(wantIouyap, string(iouyap)); d != "" {
		t.Errorf("iouyap.ini mismatch (-want +got):\n%s", d)
	}
}

func TestGenInterfaceConfigMgmtRelocated(t *testing.T) {
	n := newTestIOL(t, map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"})
	// eth15 is Ethernet0/0 under the swap rule (mgmt vacated linux index 15)
	n.Endpoints = append(n.Endpoints, vethEndpoint("eth1"), vethEndpoint("eth15"))

	if err := n.GenInterfaceConfig(t.Context()); err != nil {
		t.Fatalf("GenInterfaceConfig() unexpected error: %v", err)
	}

	netmap, err := os.ReadFile(path.Join(n.Cfg.LabDir, "NETMAP"))
	if err != nil {
		t.Fatal(err)
	}
	wantNetmap := "1:3/3 513:3/3\n1:0/1 513:0/1\n1:0/0 513:0/0\n"
	if d := cmp.Diff(wantNetmap, string(netmap)); d != "" {
		t.Errorf("NETMAP mismatch (-want +got):\n%s", d)
	}

	iouyap, err := os.ReadFile(path.Join(n.Cfg.LabDir, "iouyap.ini"))
	if err != nil {
		t.Fatal(err)
	}
	wantIouyap := "[default]\nbase_port = 49000\nnetmap = /iol/NETMAP\n" +
		"[513:3/3]\neth_dev = eth0\n" +
		"[513:0/1]\neth_dev = eth1\n" +
		"[513:0/0]\neth_dev = eth15\n"
	if d := cmp.Diff(wantIouyap, string(iouyap)); d != "" {
		t.Errorf("iouyap.ini mismatch (-want +got):\n%s", d)
	}
}

func TestCheckInterfaceName(t *testing.T) {
	tests := map[string]struct {
		env        map[string]string
		endpoints  []string
		wantErr    bool
		wantErrSub string
	}{
		"default data iface ok": {
			endpoints: []string{"eth1", "eth15"},
		},
		"default eth0 rejected": {
			endpoints:  []string{"eth0"},
			wantErr:    true,
			wantErrSub: "Ethernet0/0",
		},
		"relocated frees eth15": {
			env:       map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"},
			endpoints: []string{"eth1", "eth15"},
		},
		"relocated mgmt rejected": {
			env:        map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"},
			endpoints:  []string{"eth0"},
			wantErr:    true,
			wantErrSub: "Ethernet3/3",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			n := newTestIOL(t, tc.env)
			for _, e := range tc.endpoints {
				n.Endpoints = append(n.Endpoints, vethEndpoint(e))
			}

			err := n.CheckInterfaceName()
			if (err != nil) != tc.wantErr {
				t.Fatalf("CheckInterfaceName() error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error %q does not mention %q", err.Error(), tc.wantErrSub)
			}
		})
	}
}

func TestAddEndpointMgmtRelocated(t *testing.T) {
	n := newTestIOL(t, map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"})

	ep := vethEndpoint("Ethernet0/0")
	if err := n.AddEndpoint(ep); err != nil {
		t.Fatalf("AddEndpoint(Ethernet0/0) unexpected error: %v", err)
	}

	if d := cmp.Diff("eth15", ep.GetIfaceName()); d != "" {
		t.Errorf("iface name mismatch (-want +got):\n%s", d)
	}
	if d := cmp.Diff("Ethernet0/0", ep.GetIfaceAlias()); d != "" {
		t.Errorf("iface alias mismatch (-want +got):\n%s", d)
	}
}

func TestGenBootConfigMgmtIntf(t *testing.T) {
	tests := map[string]struct {
		env         map[string]string
		wantSubs    []string
		wantMissing []string
	}{
		"default": {
			wantSubs: []string{
				"interface Ethernet0/0",
				"ip route vrf clab-mgmt 0.0.0.0 0.0.0.0 Ethernet0/0 172.20.20.1",
			},
		},
		"relocated": {
			env: map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"},
			wantSubs: []string{
				"interface Ethernet3/3",
				"ip route vrf clab-mgmt 0.0.0.0 0.0.0.0 Ethernet3/3 172.20.20.1",
			},
			wantMissing: []string{"interface Ethernet0/0"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			n := newTestIOL(t, tc.env)
			n.Cfg.MgmtIPv4Address = "172.20.20.2"
			n.Cfg.MgmtIPv4PrefixLength = 24
			n.Cfg.MgmtIPv4Gateway = "172.20.20.1"
			n.Cfg.MgmtIPv6Address = "3fff:172:20:20::2"
			n.Cfg.MgmtIPv6PrefixLength = 64
			n.Cfg.MgmtIPv6Gateway = "3fff:172:20:20::1"

			if err := n.GenBootConfig(t.Context()); err != nil {
				t.Fatalf("GenBootConfig() unexpected error: %v", err)
			}

			cfg, err := os.ReadFile(path.Join(n.Cfg.LabDir, "boot_config.txt"))
			if err != nil {
				t.Fatal(err)
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

func TestEnsureNumSlotsEnv(t *testing.T) {
	tests := map[string]struct {
		env       map[string]string
		endpoints []string
		wantEnv   string
	}{
		"default mgmt leaves env unset": {
			endpoints: []string{"eth1"},
			wantEnv:   "",
		},
		"relocated sets env to cover mgmt slot": {
			env:       map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet3/3"},
			endpoints: []string{"eth1"},
			wantEnv:   "4",
		},
		"relocated respects larger data slot": {
			env:       map[string]string{"CLAB_IOL_MGMT_INTF": "Ethernet1/1"},
			endpoints: []string{"eth9"},
			wantEnv:   "3",
		},
		"user value kept": {
			env: map[string]string{
				"CLAB_IOL_MGMT_INTF": "Ethernet1/1",
				"CLAB_IOL_NUM_SLOTS": "8",
			},
			endpoints: []string{"eth1"},
			wantEnv:   "8",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			n := newTestIOL(t, tc.env)
			for _, e := range tc.endpoints {
				n.Endpoints = append(n.Endpoints, vethEndpoint(e))
			}

			n.ensureNumSlotsEnv()

			if d := cmp.Diff(tc.wantEnv, n.Cfg.Env["CLAB_IOL_NUM_SLOTS"]); d != "" {
				t.Errorf("CLAB_IOL_NUM_SLOTS mismatch (-want +got):\n%s", d)
			}
		})
	}
}
