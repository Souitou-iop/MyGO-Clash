package secure

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// DefaultKeyring returns the login Keychain, through the security tool.
func DefaultKeyring(service, dataDir string) Keyring {
	if forceFile() {
		return FileKeyring(dataDir)
	}
	return keychain{service: service}
}

type keychain struct{ service string }

func (k keychain) Get(account string) ([]byte, error) {
	cmd := exec.Command("/usr/bin/security", "find-generic-password", "-s", k.service, "-a", account, "-w")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 44 { // errSecItemNotFound
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("keychain: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
}

// Set writes the secret through security's interactive mode, from its
// standard input, so that it never shows in the list of processes.
func (k keychain) Set(account string, secret []byte) error {
	line := fmt.Sprintf("add-generic-password -U -s %s -a %s -l %s -w %s\n",
		quote(k.service), quote(account), quote(k.service+" ("+account+")"), base64.StdEncoding.EncodeToString(secret))
	cmd := exec.Command("/usr/bin/security", "-i")
	cmd.Stdin = strings.NewReader(line)
	out, err := cmd.CombinedOutput()
	if err != nil || bytes.Contains(out, []byte("rror")) {
		return fmt.Errorf("keychain: %v: %s", err, strings.TrimSpace(string(out)))
	}
	if _, err := k.Get(account); err != nil {
		return fmt.Errorf("keychain: the secret did not save: %w", err)
	}
	return nil
}

func (k keychain) Delete(account string) error {
	err := exec.Command("/usr/bin/security", "delete-generic-password", "-s", k.service, "-a", account).Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 44 {
		return nil
	}
	return err
}

func (keychain) Name() string { return "keychain" }
func (keychain) Secure() bool { return true }

func quote(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
