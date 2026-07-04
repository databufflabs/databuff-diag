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
		{name: "uname", cmd: "uname -r", want: RiskReadonly},
		{name: "cat", cmd: "cat /etc/passwd", want: RiskReadonly},
		{name: "grep", cmd: "grep ERROR /var/log/syslog", want: RiskReadonly},
		{name: "docker ps", cmd: "docker ps -a", want: RiskReadonly},
		{name: "docker logs", cmd: "docker logs lmnr-frontend-1 --tail 50", want: RiskReadonly},
		{name: "docker inspect", cmd: "docker inspect ai-apm-demo --format '{{.State.Health.Status}}'", want: RiskReadonly},
		{name: "docker info", cmd: "docker info 2>&1 | head -20", want: RiskReadonly},
		{name: "docker events", cmd: "docker events --since 10m --until 0s", want: RiskReadonly},
		{name: "docker compose ps", cmd: "docker compose -f /opt/databuff-ai-apm/docker-compose.yml ps -a", want: RiskReadonly},
		{name: "docker compose logs", cmd: "docker compose -f /opt/databuff-ai-apm/docker-compose.yml logs ai-apm-web --tail 80", want: RiskReadonly},
		{name: "docker compose config", cmd: "docker compose -f /opt/databuff-ai-apm/docker-compose.yml config --services", want: RiskReadonly},
		{name: "kubectl get", cmd: "kubectl get pods -n default", want: RiskReadonly},
		{name: "kubectl get with global flags", cmd: "kubectl -n kube-system get pods -o wide", want: RiskReadonly},
		{name: "kubectl describe", cmd: "kubectl describe pod foo -n default", want: RiskReadonly},
		{name: "kubectl logs", cmd: "kubectl logs -n default my-pod --previous --tail=80", want: RiskReadonly},
		{name: "kubectl get events", cmd: "kubectl get events -A --sort-by='.lastTimestamp' | tail -30", want: RiskReadonly},
		{name: "kubectl auth can-i", cmd: "kubectl auth can-i get pods", want: RiskReadonly},
		{name: "kubectl config view", cmd: "kubectl config view --minify", want: RiskReadonly},
		{name: "kubectl rollout status", cmd: "kubectl rollout status deployment/api", want: RiskReadonly},
		{name: "systemctl status", cmd: "systemctl status nginx --no-pager", want: RiskReadonly},
		{name: "ss listen ports", cmd: "ss -tlnp 2>/dev/null | grep ':27403'", want: RiskReadonly},
		{name: "du disk usage", cmd: "du -xh --max-depth=1 /var /tmp /home 2>/dev/null | sort -hr | head -15", want: RiskReadonly},
		{name: "ps aux", cmd: "ps aux | grep nginx", want: RiskReadonly},
		{name: "ping", cmd: "ping -c 3 127.0.0.1", want: RiskReadonly},
		{name: "curl", cmd: "curl -s http://localhost:8787/health", want: RiskReadonly},
		{name: "journalctl", cmd: "journalctl -u nginx --no-pager", want: RiskReadonly},
		{name: "df", cmd: "df -h", want: RiskReadonly},
		{name: "free", cmd: "free -m", want: RiskReadonly},
		{name: "cd", cmd: "cd /tmp && pwd", want: RiskReadonly},
		{name: "git log", cmd: "git log --oneline -10", want: RiskReadonly},
		{name: "git remote", cmd: "git remote -v", want: RiskReadonly},
		{name: "cd git log chain", cmd: "cd /tmp/repo && git log --oneline -5", want: RiskReadonly},
		{name: "test file exists", cmd: `[ -f "/tmp/README.md" ]`, want: RiskReadonly},
		{name: "cd if head chain", cmd: `cd /tmp/repo && if [ -f "README.md" ]; then head -100 README.md; fi`, want: RiskReadonly},
		{name: "find exec echo", cmd: `find /tmp/packages -name "CHANGELOG.md" -exec echo "{}" \;`, want: RiskWrite},
		{name: "find pipe grep", cmd: `find /tmp/packages -name "*.ts" -path "*/src/*" | grep foo`, want: RiskReadonly},

		// write
		{name: "systemctl restart", cmd: "systemctl restart nginx", want: RiskWrite},
		{name: "docker compose", cmd: "docker compose up -d", want: RiskWrite},
		{name: "docker compose down", cmd: "docker compose down", want: RiskWrite},
		{name: "docker restart", cmd: "docker restart lmnr-frontend-1", want: RiskWrite},
		{name: "docker exec", cmd: "docker exec -it lmnr-frontend-1 sh", want: RiskWrite},
		{name: "kubectl apply", cmd: "kubectl apply -f deployment.yaml", want: RiskWrite},
		{name: "kubectl delete", cmd: "kubectl delete pod foo", want: RiskWrite},
		{name: "kubectl exec", cmd: "kubectl exec -it foo -- sh", want: RiskWrite},
		{name: "kubectl rollout restart", cmd: "kubectl rollout restart deployment/api", want: RiskWrite},
		{name: "kubectl config set-context", cmd: "kubectl config set-context prod", want: RiskWrite},
		{name: "helm upgrade", cmd: "helm upgrade api ./chart", want: RiskWrite},
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
