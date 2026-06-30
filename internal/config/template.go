package config

import (
	"fmt"
	"strings"
)

// StarterTemplate returns the contents of a minimal starter .lunarforge.yml for
// a project with the given name. It uses a single verify script.
func StarterTemplate(projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "my-repo"
	}
	return fmt.Sprintf(`version: 1

project:
  name: %s

verify:
  commands:
    - id: verify
      run: ./scripts/verify.sh

explain:
  agent: claude
  command: claude
  args:
    - --print
    - --permission-mode
    - plan

evidence:
  dir: .lf/runs
  require_fresh_diff: true
`, name)
}
