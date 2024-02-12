package managers

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	lxd_shared "github.com/canonical/lxd/shared"

	"github.com/canonical/lxd-imagebuilder/shared"
)

type apt struct {
	common
}

func (m *apt) load() error {
	m.commands = managerCommands{
		clean:   "apt-get",
		install: "apt-get",
		refresh: "apt-get",
		remove:  "apt-get",
		update:  "apt-get",
	}

	m.flags = managerFlags{
		clean: []string{
			"clean",
		},
		global: []string{
			"-y",
		},
		install: []string{
			"install",
		},
		remove: []string{
			"remove", "--auto-remove",
		},
		refresh: []string{
			"update",
		},
		update: []string{
			"dist-upgrade",
		},
	}

	return nil
}

func (m *apt) manageRepository(repoAction shared.DefinitionPackagesRepository) error {
	var targetFile string

	if repoAction.Name == "sources.list" {
		targetFile = filepath.Join("/etc/apt", repoAction.Name)
	} else {
		targetFile = filepath.Join("/etc/apt/sources.list.d", repoAction.Name)

		if !strings.HasSuffix(targetFile, ".list") {
			targetFile = fmt.Sprintf("%s.list", targetFile)
		}
	}

	if !lxd_shared.PathExists(filepath.Dir(targetFile)) {
		err := os.MkdirAll(filepath.Dir(targetFile), 0755)
		if err != nil {
			return fmt.Errorf("Failed to create directory %q: %w", filepath.Dir(targetFile), err)
		}
	}

	f, err := os.OpenFile(targetFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("Failed to open file %q: %w", targetFile, err)
	}

	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("Failed to read from file %q: %w", targetFile, err)
	}

	// Truncate file if it's not generated by distrobuilder
	if !strings.HasPrefix(string(content), "# Generated by distrobuilder\n") {
		err = f.Truncate(0)
		if err != nil {
			return fmt.Errorf("Failed to truncate %q: %w", targetFile, err)
		}

		_, err = f.Seek(0, 0)
		if err != nil {
			return fmt.Errorf("Failed to seek on file %q: %w", targetFile, err)
		}

		_, err = f.WriteString("# Generated by distrobuilder\n")
		if err != nil {
			return fmt.Errorf("Failed to write to file %q: %w", targetFile, err)
		}
	}

	_, err = f.WriteString(repoAction.URL)
	if err != nil {
		return fmt.Errorf("Failed to write to file %q: %w", targetFile, err)
	}

	// Append final new line if missing
	if !strings.HasSuffix(repoAction.URL, "\n") {
		_, err = f.WriteString("\n")
		if err != nil {
			return fmt.Errorf("Failed to write to file %q: %w", targetFile, err)
		}
	}

	if repoAction.Key != "" {
		var reader io.Reader

		if strings.HasPrefix(repoAction.Key, "-----BEGIN PGP PUBLIC KEY BLOCK-----") {
			reader = strings.NewReader(repoAction.Key)
		} else {
			// If only key ID is provided, we need gpg to be installed early.
			err := shared.RunCommand(m.ctx, nil, nil, "gpg", "--recv-keys", repoAction.Key)
			if err != nil {
				return fmt.Errorf("Failed to receive GPG keys: %w", err)
			}

			var buf bytes.Buffer

			err = shared.RunCommand(m.ctx, nil, &buf, "gpg", "--export", "--armor", repoAction.Key)
			if err != nil {
				return fmt.Errorf("Failed to export GPG keys: %w", err)
			}

			reader = &buf
		}

		signatureFilePath := filepath.Join("/etc/apt/trusted.gpg.d", fmt.Sprintf("%s.asc", repoAction.Name))

		f, err := os.Create(signatureFilePath)
		if err != nil {
			return fmt.Errorf("Failed to create file %q: %w", signatureFilePath, err)
		}

		defer f.Close()

		_, err = io.Copy(f, reader)
		if err != nil {
			return fmt.Errorf("Failed to copy file: %w", err)
		}
	}

	return nil
}
