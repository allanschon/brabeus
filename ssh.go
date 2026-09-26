package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// sshCommandFor builds the GIT_SSH_COMMAND for one store.
//
// Each store gets its own key: the record is written to, the mirror is only
// read. IdentitiesOnly is not optional — without it ssh offers every key it can
// find and may authenticate as something with more authority than intended.
//
// accept-new is trust on first use, and in a container whose home is
// ephemeral "first use" would be every restart. known_hosts lives on the data
// volume so the key accepted once is the key checked ever after. The port is
// not here: it belongs to the repository URL, which git hands to ssh itself.
func sshCommandFor(keyPath, knownHosts string) string {
	return fmt.Sprintf(
		"ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=%s",
		keyPath, knownHosts)
}

// prepareKnownHosts makes sure the directory known_hosts lives in exists. ssh
// does not create it, and without it the pin is silently defeated: ssh prints
// a warning that git swallows, and trust on first use happens on every use.
func prepareKnownHosts(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o750)
}
