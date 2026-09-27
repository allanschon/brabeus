package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Each store gets its own key: the record is written to, the mirror must only
// be read. Building the command from the wrong path is a silent failure - the
// clone simply fails, or worse, succeeds with more authority than intended.
func TestSSHCommandNamesTheKeyItWasGiven(t *testing.T) {
	got := SSHCommandFor("/etc/brabeus/projects_key", "/var/lib/brabeus/known_hosts")
	if !strings.Contains(got, "-i /etc/brabeus/projects_key") {
		t.Errorf("command does not use the given key: %q", got)
	}
	// Without IdentitiesOnly, ssh offers every key the agent holds and may
	// authenticate as something other than the key we named.
	if !strings.Contains(got, "IdentitiesOnly=yes") {
		t.Errorf("IdentitiesOnly missing: %q", got)
	}
	// The port belongs to the repository URL, which git passes to ssh itself.
	// A second copy here would silently win over a URL that says otherwise.
	if strings.Contains(got, "-p ") {
		t.Errorf("port hard-coded in the ssh command: %q", got)
	}
}

// accept-new is trust on first use. In a container whose home is ephemeral,
// "first use" is every restart, and the host key is never actually checked.
// The known_hosts file has to live on the data volume to mean anything.
func TestSSHCommandPersistsKnownHostsWhereItWasTold(t *testing.T) {
	got := SSHCommandFor("/etc/brabeus/deploy_key", "/var/lib/brabeus/known_hosts")
	if !strings.Contains(got, "UserKnownHostsFile=/var/lib/brabeus/known_hosts") {
		t.Errorf("known_hosts not pinned to the data volume: %q", got)
	}
	if !strings.Contains(got, "StrictHostKeyChecking=accept-new") {
		t.Errorf("host key checking missing: %q", got)
	}
}

func TestSSHCommandsForDifferentKeysDiffer(t *testing.T) {
	a := SSHCommandFor("/etc/brabeus/deploy_key", "/var/lib/brabeus/known_hosts")
	b := SSHCommandFor("/etc/brabeus/projects_key", "/var/lib/brabeus/known_hosts")
	if a == b {
		t.Error("two different keys produced the same ssh command")
	}
}

// ssh does not create the directory known_hosts lives in. Without it the pin
// is silently defeated: ssh prints a warning git swallows, and trust on first
// use happens on every use.
func TestPrepareKnownHostsCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "known_hosts")
	if err := PrepareKnownHosts(path); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(filepath.Dir(path)); err != nil || !st.IsDir() {
		t.Errorf("directory for %s was not created: %v", path, err)
	}
}
