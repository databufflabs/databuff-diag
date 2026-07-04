package policy

import "testing"

func TestEngine_Classify(t *testing.T) {
	engine := &Engine{}

	tests := []struct {
		name string
		cmd  string
		want Risk
	}{
		// readonly
		{name: "ls", cmd: "ls -la /var/log", want: RiskReadonly},
		{name: "cat", cmd: "cat /etc/passwd", want: RiskReadonly},
		{name: "grep", cmd: "grep ERROR /var/log/syslog", want: RiskReadonly},
		{name: "docker ps", cmd: "docker ps -a", want: RiskReadonly},
		{name: "kubectl get", cmd: "kubectl get pods -n default", want: RiskReadonly},
		{name: "curl", cmd: "curl -s http://localhost:8787/health", want: RiskReadonly},
		{name: "journalctl", cmd: "journalctl -u nginx --no-pager", want: RiskReadonly},
		{name: "df", cmd: "df -h", want: RiskReadonly},
		{name: "free", cmd: "free -m", want: RiskReadonly},

		// write
		{name: "systemctl restart", cmd: "systemctl restart nginx", want: RiskWrite},
		{name: "docker compose", cmd: "docker compose up -d", want: RiskWrite},
		{name: "kubectl apply", cmd: "kubectl apply -f deployment.yaml", want: RiskWrite},
		{name: "kubectl delete", cmd: "kubectl delete pod foo", want: RiskWrite},
		{name: "sed in-place", cmd: `sed -i 's/foo/bar/' config.yaml`, want: RiskWrite},

		// dangerous
		{name: "rm -rf tmp", cmd: "rm -rf /tmp/cache", want: RiskDangerous},
		{name: "mkfs", cmd: "mkfs.ext4 /dev/sdb1", want: RiskDangerous},
		{name: "dd", cmd: "dd if=/dev/zero of=/dev/sda bs=1M", want: RiskDangerous},
		{name: "iptables flush", cmd: "iptables -F", want: RiskDangerous},

		// blocked (blacklist)
		{name: "rm -rf root", cmd: "rm -rf /", want: RiskBlocked},
		{name: "fork bomb", cmd: ":(){ :|:& };:", want: RiskBlocked},

		// pipes
		{name: "pipe readonly", cmd: "ls -la | grep nginx", want: RiskReadonly},
		{name: "pipe write via tee", cmd: "echo hello | tee /tmp/out.txt", want: RiskWrite},
		{name: "pipe dangerous", cmd: "ls | rm -rf /var/tmp/old", want: RiskDangerous},

		// subshells
		{name: "subshell blocked", cmd: "(rm -rf /)", want: RiskBlocked},
		{name: "subshell write", cmd: "(systemctl restart docker)", want: RiskWrite},
		{name: "cmd subst dangerous", cmd: "echo $(rm -rf /opt/legacy)", want: RiskDangerous},

		// redirect write
		{name: "redirect write", cmd: "echo data > /tmp/output.txt", want: RiskWrite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := engine.Classify(tt.cmd)
			if err != nil {
				t.Fatalf("Classify() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Classify(%q) = %q, want %q", tt.cmd, got, tt.want)
			}
		})
	}
}

func TestNeedsApproval(t *testing.T) {
	tests := []struct {
		risk Risk
		mode Mode
		want bool
	}{
		{RiskReadonly, AllApproval, true},
		{RiskReadonly, WriteApproval, false},
		{RiskReadonly, Open, false},

		{RiskWrite, AllApproval, true},
		{RiskWrite, WriteApproval, true},
		{RiskWrite, Open, false},

		{RiskDangerous, AllApproval, true},
		{RiskDangerous, WriteApproval, true},
		{RiskDangerous, Open, true},

		{RiskBlocked, AllApproval, false},
		{RiskBlocked, WriteApproval, false},
		{RiskBlocked, Open, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.risk)+"/"+string(tt.mode), func(t *testing.T) {
			if got := NeedsApproval(tt.risk, tt.mode); got != tt.want {
				t.Errorf("NeedsApproval(%q, %q) = %v, want %v", tt.risk, tt.mode, got, tt.want)
			}
		})
	}
}
