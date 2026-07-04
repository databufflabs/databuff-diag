package policy

import (
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Risk is the classified risk level of a shell command.
type Risk string

const (
	RiskReadonly  Risk = "readonly"
	RiskWrite     Risk = "write"
	RiskDangerous Risk = "dangerous"
	RiskBlocked   Risk = "blocked"
)

// Engine classifies shell commands using an AST built by mvdan/sh.
type Engine struct{}

var (
	blacklistPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)rm\s+.*(-rf|-fr)\s+/?\s*$`),
		regexp.MustCompile(`(?i)rm\s+(-rf|-fr)\s+/\s`),
		regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`),
	}

	forkBombPattern = regexp.MustCompile(`:\s*\(\s*\)\s*\{`)

	dockerReadonlySubcmds = map[string]bool{
		"ps": true, "logs": true, "inspect": true, "info": true, "version": true,
		"events": true, "stats": true, "top": true, "port": true, "diff": true,
		"images": true, "history": true, "search": true,
	}
	dockerWriteSubcmds = map[string]bool{
		"run": true, "exec": true, "start": true, "stop": true, "restart": true,
		"kill": true, "rm": true, "rmi": true, "build": true, "pull": true,
		"push": true, "commit": true, "create": true, "update": true, "cp": true,
		"export": true, "import": true, "load": true, "save": true, "tag": true,
		"untag": true, "login": true, "logout": true, "prune": true,
	}
	composeReadonlySubcmds = map[string]bool{
		"ps": true, "logs": true, "config": true, "top": true, "port": true,
		"images": true, "ls": true, "version": true,
	}
	composeWriteSubcmds = map[string]bool{
		"up": true, "down": true, "stop": true, "start": true, "restart": true,
		"rm": true, "build": true, "pull": true, "push": true, "exec": true,
		"run": true, "create": true, "kill": true, "cp": true, "pause": true,
		"unpause": true,
	}

	readonlyCommands = map[string]bool{
		"ls": true, "cat": true, "grep": true, "egrep": true, "fgrep": true, "zgrep": true,
		"head": true, "tail": true, "less": true, "more": true, "wc": true, "sort": true,
		"uniq": true, "cut": true, "tr": true, "awk": true, "sed": true, "file": true,
		"stat": true, "readlink": true, "basename": true, "dirname": true,
		"uname": true, "uptime": true, "whoami": true, "hostname": true, "id": true,
		"env": true, "printenv": true, "date": true, "dmesg": true,
		"df": true, "free": true, "du": true, "ps": true, "pgrep": true, "top": true,
		"ss": true, "netstat": true, "lsof": true, "ping": true, "dig": true,
		"nslookup": true, "host": true, "curl": true, "wget": true,
		"journalctl": true, "findmnt": true, "mount": true, "which": true, "type": true,
		"command": true, "pwd": true, "echo": true, "true": true, "false": true,
		"[": true, "test": true,
		"cd": true,
		"find": true, "strings": true, "xxd": true, "od": true, "lsblk": true,
		"vmstat": true, "iostat": true, "mpstat": true, "sar": true,
	}

	kubectlReadonlySubcmds = map[string]bool{
		"get": true, "describe": true, "logs": true, "top": true, "explain": true,
		"api-resources": true, "api-versions": true, "cluster-info": true, "version": true,
		"wait": true, "port-forward": true,
	}
	kubectlWriteSubcmds = map[string]bool{
		"apply": true, "delete": true, "create": true, "patch": true, "replace": true,
		"scale": true, "edit": true, "exec": true, "attach": true,
		"cp": true, "run": true, "expose": true, "label": true, "annotate": true,
		"taint": true, "cordon": true, "uncordon": true, "drain": true, "set": true,
	}

	kubectlConfigReadonlySubcmds = map[string]bool{
		"view": true, "get-contexts": true, "current-context": true,
	}
	kubectlRolloutReadonlySubcmds = map[string]bool{
		"status": true, "history": true,
	}
	kubectlRolloutWriteSubcmds = map[string]bool{
		"restart": true, "undo": true, "pause": true, "resume": true,
	}

	systemctlReadonlySubcmds = map[string]bool{
		"status": true, "is-active": true, "is-enabled": true, "is-failed": true,
		"show": true, "list-units": true, "list-unit-files": true, "list-jobs": true,
		"cat": true, "help": true,
	}
	systemctlWriteSubcmds = map[string]bool{
		"start": true, "stop": true, "restart": true, "reload": true,
		"enable": true, "disable": true, "mask": true, "unmask": true,
		"reset-failed": true, "daemon-reload": true,
	}

	helmReadonlySubcmds = map[string]bool{
		"list": true, "status": true, "get": true, "history": true, "version": true,
	}
	helmWriteSubcmds = map[string]bool{
		"install": true, "upgrade": true, "uninstall": true, "rollback": true, "delete": true,
	}

	gitReadonlySubcmds = map[string]bool{
		"log": true, "show": true, "status": true, "diff": true, "remote": true,
		"branch": true, "tag": true, "rev-parse": true, "describe": true,
		"shortlog": true, "blame": true, "whatchanged": true, "ls-files": true,
		"ls-remote": true, "grep": true, "name-rev": true, "version": true,
	}
	gitWriteSubcmds = map[string]bool{
		"add": true, "commit": true, "push": true, "pull": true, "merge": true,
		"rebase": true, "checkout": true, "switch": true, "reset": true, "clean": true,
		"rm": true, "mv": true, "stash": true, "cherry-pick": true, "revert": true,
		"clone": true, "init": true, "fetch": true,
	}

	kubectlValueFlags = map[string]bool{
		"-n": true, "--namespace": true, "--context": true, "--cluster": true,
		"--user": true, "--kubeconfig": true, "--field-selector": true,
		"-l": true, "--selector": true, "--sort-by": true, "--tail": true,
		"--since": true, "--container": true, "-o": true, "--output": true,
		"--grace-period": true, "--timeout": true, "-f": true, "--filename": true,
		"--for": true, "--revision": true,
	}

	systemctlValueFlags = map[string]bool{
		"--user": true, "--type": true, "--state": true, "--property": true,
	}
)

// Classify parses cmd and returns the highest risk found across pipelines,
// subshells, and command substitutions.
func (e *Engine) Classify(cmd string) (Risk, error) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return RiskReadonly, nil
	}

	if matchesBlacklist(cmd) {
		return RiskBlocked, nil
	}

	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(cmd), "")
	if err != nil {
		return classifyHeuristic(cmd), nil
	}

	risk := RiskReadonly
	syntax.Walk(file, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			risk = maxRisk(risk, classifyCall(n))
		case *syntax.Redirect:
			if isWriteRedirect(n) {
				risk = maxRisk(risk, RiskWrite)
			}
		}
		return true
	})

	if risk != RiskBlocked && matchesBlacklist(cmd) {
		return RiskBlocked, nil
	}

	return risk, nil
}

func matchesBlacklist(cmd string) bool {
	for _, re := range blacklistPatterns {
		if re.MatchString(cmd) {
			return true
		}
	}
	return false
}

func classifyHeuristic(cmd string) Risk {
	lower := strings.ToLower(cmd)
	if matchesBlacklist(cmd) || forkBombPattern.MatchString(cmd) {
		return RiskBlocked
	}
	if strings.Contains(lower, "rm -rf") || strings.Contains(lower, "rm -fr") {
		return RiskDangerous
	}
	if strings.Contains(lower, "mkfs") || strings.Contains(lower, " dd ") || strings.HasPrefix(lower, "dd ") {
		return RiskDangerous
	}
	if strings.Contains(lower, "iptables -f") || strings.Contains(lower, "iptables -F") {
		return RiskDangerous
	}
	if strings.Contains(lower, "sed -i") || strings.Contains(lower, "systemctl restart") ||
		strings.Contains(lower, "systemctl stop") || strings.Contains(lower, "systemctl start") {
		return RiskWrite
	}
	if words := kubectlWordsFromHeuristic(lower); len(words) > 0 {
		return classifyKubectl(words)
	}
	if words := systemctlWordsFromHeuristic(lower); len(words) > 0 {
		return classifySystemctl(words)
	}
	if words := helmWordsFromHeuristic(lower); len(words) > 0 {
		return classifyHelm(words)
	}
	if strings.Contains(lower, "docker compose") {
		sub := heuristicComposeSubcommand(lower)
		if sub != "" {
			if composeWriteSubcmds[sub] {
				return RiskWrite
			}
			if composeReadonlySubcmds[sub] {
				return RiskReadonly
			}
		}
		return RiskWrite
	}
	for sub := range dockerWriteSubcmds {
		if strings.Contains(lower, "docker "+sub) {
			return RiskWrite
		}
	}
	for sub := range dockerReadonlySubcmds {
		if strings.Contains(lower, "docker "+sub) {
			return RiskReadonly
		}
	}
	for cmd := range readonlyCommands {
		if strings.HasPrefix(lower, cmd+" ") || lower == cmd {
			return RiskReadonly
		}
	}
	return RiskWrite
}

func classifyCall(call *syntax.CallExpr) Risk {
	words := callWords(call)
	if len(words) == 0 {
		return RiskReadonly
	}

	full := strings.Join(words, " ")
	if matchesBlacklist(full) || forkBombPattern.MatchString(full) {
		return RiskBlocked
	}

	cmd := strings.ToLower(words[0])

	if strings.HasPrefix(cmd, "mkfs") {
		return RiskDangerous
	}

	switch cmd {
	case "docker":
		return classifyDocker(words)
	case "kubectl":
		return classifyKubectl(words)
	case "systemctl":
		return classifySystemctl(words)
	case "helm":
		return classifyHelm(words)
	case "git":
		return classifyGit(words)
	case "tee":
		return RiskWrite
	case "rm":
		if hasFlag(words, "-rf", "-fr") {
			if targetsRoot(words) {
				return RiskBlocked
			}
			return RiskDangerous
		}
	case "dd":
		return RiskDangerous
	case "iptables":
		if len(words) > 1 && strings.EqualFold(words[1], "-F") {
			return RiskDangerous
		}
	}

	if readonlyCommands[cmd] {
		return classifyReadonlyCommand(cmd, words)
	}

	return RiskWrite
}

func classifyReadonlyCommand(cmd string, words []string) Risk {
	switch cmd {
	case "sed":
		for _, w := range words[1:] {
			if strings.HasPrefix(w, "-i") {
				return RiskWrite
			}
		}
	case "find":
		for _, w := range words[1:] {
			switch w {
			case "-delete", "-execdir", "-ok":
				return RiskWrite
			}
			if strings.HasPrefix(w, "-exec") {
				return RiskWrite
			}
		}
	case "wget":
		for _, w := range words[1:] {
			if w == "-O" || strings.HasPrefix(w, "-O") {
				return RiskWrite
			}
		}
	}
	return RiskReadonly
}

func classifyKubectl(words []string) Risk {
	sub := subcommandAt(words, 1, kubectlValueFlags, 0)
	if sub == "" {
		return RiskWrite
	}
	switch sub {
	case "config":
		nested := subcommandAt(words, 1, kubectlValueFlags, 1)
		return classifySubcommandRisk(nested, kubectlConfigReadonlySubcmds, nil)
	case "rollout":
		nested := subcommandAt(words, 1, kubectlValueFlags, 1)
		return classifySubcommandRisk(nested, kubectlRolloutReadonlySubcmds, kubectlRolloutWriteSubcmds)
	case "auth":
		nested := subcommandAt(words, 1, kubectlValueFlags, 1)
		if nested == "can-i" {
			return RiskReadonly
		}
		return RiskWrite
	}
	return classifySubcommandRisk(sub, kubectlReadonlySubcmds, kubectlWriteSubcmds)
}

func classifySystemctl(words []string) Risk {
	sub := firstSubcommand(words, 1, systemctlValueFlags)
	return classifySubcommandRisk(sub, systemctlReadonlySubcmds, systemctlWriteSubcmds)
}

func classifyHelm(words []string) Risk {
	sub := firstSubcommand(words, 1, nil)
	return classifySubcommandRisk(sub, helmReadonlySubcmds, helmWriteSubcmds)
}

func classifyGit(words []string) Risk {
	sub := firstSubcommand(words, 1, gitValueFlags)
	return classifySubcommandRisk(sub, gitReadonlySubcmds, gitWriteSubcmds)
}

var gitValueFlags = map[string]bool{
	"-C": true, "--git-dir": true, "--work-tree": true, "--namespace": true,
}

func classifySubcommandRisk(sub string, readonly, write map[string]bool) Risk {
	if sub == "" {
		return RiskWrite
	}
	if readonly[sub] {
		return RiskReadonly
	}
	if write[sub] {
		return RiskWrite
	}
	return RiskWrite
}

func classifyDocker(words []string) Risk {
	if len(words) < 2 {
		return RiskWrite
	}
	sub := strings.ToLower(words[1])
	if sub == "compose" {
		return classifyDockerCompose(words)
	}
	if dockerReadonlySubcmds[sub] {
		return RiskReadonly
	}
	if dockerWriteSubcmds[sub] {
		return RiskWrite
	}
	return RiskWrite
}

func classifyDockerCompose(words []string) Risk {
	sub := firstSubcommand(words, 2, composeValueFlags)
	if sub == "" {
		return RiskWrite
	}
	if composeReadonlySubcmds[sub] {
		return RiskReadonly
	}
	if composeWriteSubcmds[sub] {
		return RiskWrite
	}
	return RiskWrite
}

var composeValueFlags = map[string]bool{
	"-f": true, "--file": true,
	"-p": true, "--project-name": true,
	"--project-directory": true,
	"--env-file": true,
}

func firstSubcommand(words []string, start int, valueFlags map[string]bool) string {
	return subcommandAt(words, start, valueFlags, 0)
}

func subcommandAt(words []string, start int, valueFlags map[string]bool, index int) string {
	seen := 0
	for i := start; i < len(words); i++ {
		w := words[i]
		if !strings.HasPrefix(w, "-") {
			if seen == index {
				return strings.ToLower(w)
			}
			seen++
			continue
		}
		flag := strings.SplitN(w, "=", 2)[0]
		if valueFlags != nil && valueFlags[flag] && !strings.Contains(w, "=") && i+1 < len(words) {
			i++
		}
	}
	return ""
}

func callWords(call *syntax.CallExpr) []string {
	var words []string
	for _, arg := range call.Args {
		if lit := arg.Lit(); lit != "" {
			words = append(words, lit)
		}
	}
	return words
}

func hasFlag(words []string, flags ...string) bool {
	for _, w := range words[1:] {
		for _, f := range flags {
			if w == f || strings.HasPrefix(w, f) {
				return true
			}
		}
	}
	return false
}

func targetsRoot(words []string) bool {
	for _, w := range words[1:] {
		if strings.HasPrefix(w, "-") {
			continue
		}
		clean := strings.TrimSpace(w)
		if clean == "/" || clean == "/*" {
			return true
		}
	}
	return false
}

func isWriteRedirect(r *syntax.Redirect) bool {
	if r.Word != nil {
		target := strings.TrimSpace(r.Word.Lit())
		if target == "/dev/null" {
			return false
		}
	}
	if r.Op == syntax.DplOut && r.Word != nil {
		target := strings.TrimSpace(r.Word.Lit())
		if target != "" && isFileDescriptor(target) {
			return false
		}
	}
	switch r.Op {
	case syntax.RdrOut, syntax.AppOut, syntax.DplOut, syntax.RdrInOut, syntax.ClbOut, syntax.RdrAll, syntax.AppAll:
		return true
	default:
		return false
	}
}

func isFileDescriptor(s string) bool {
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return s != ""
}

func heuristicComposeSubcommand(lower string) string {
	const prefix = "docker compose"
	idx := strings.Index(lower, prefix)
	if idx < 0 {
		return ""
	}
	words := strings.Fields(strings.TrimSpace(lower[idx+len(prefix):]))
	return firstSubcommand(words, 0, composeValueFlags)
}

func kubectlWordsFromHeuristic(lower string) []string {
	idx := strings.Index(lower, "kubectl")
	if idx < 0 {
		return nil
	}
	return strings.Fields(strings.TrimSpace(lower[idx:]))
}

func systemctlWordsFromHeuristic(lower string) []string {
	idx := strings.Index(lower, "systemctl")
	if idx < 0 {
		return nil
	}
	return strings.Fields(strings.TrimSpace(lower[idx:]))
}

func helmWordsFromHeuristic(lower string) []string {
	idx := strings.Index(lower, "helm")
	if idx < 0 {
		return nil
	}
	return strings.Fields(strings.TrimSpace(lower[idx:]))
}

func maxRisk(a, b Risk) Risk {
	order := map[Risk]int{
		RiskReadonly:  0,
		RiskWrite:     1,
		RiskDangerous: 2,
		RiskBlocked:   3,
	}
	if order[a] >= order[b] {
		return a
	}
	return b
}
