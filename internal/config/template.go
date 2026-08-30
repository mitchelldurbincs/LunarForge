package config

// StarterTemplate returns the deliberately small repository policy written by
// `lf init`. Repositories may add more named checks as they need them.
func StarterTemplate() string {
	return `version: 1

verify:
  commands:
    - id: verify
      run: ./scripts/verify.sh
`
}
