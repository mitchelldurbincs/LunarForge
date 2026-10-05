package hooks

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
)

// ValidatePrePush consumes Git's complete stdin ref list before checking the
// single supported operation: one branch update whose local object is HEAD.
// Deletions, tags, empty input, malformed input, and multi-ref pushes fail closed.
func ValidatePrePush(input io.Reader, head string) error {
	ref, err := readPrePushRef(input)
	if err != nil {
		return err
	}
	for _, oid := range []string{ref[1], ref[3]} {
		if len(oid) != len(head) {
			return fmt.Errorf("pre-push: malformed object ID")
		}
		if _, err := hex.DecodeString(oid); err != nil {
			return fmt.Errorf("pre-push: malformed object ID")
		}
	}
	if strings.Trim(ref[1], "0") == "" || ref[0] == "(delete)" {
		return fmt.Errorf("pre-push: deletion updates are not supported")
	}
	if strings.HasPrefix(ref[0], "refs/tags/") || strings.HasPrefix(ref[2], "refs/tags/") {
		return fmt.Errorf("pre-push: tag updates are not supported")
	}
	if (ref[0] != "HEAD" && !strings.HasPrefix(ref[0], "refs/heads/")) || !strings.HasPrefix(ref[2], "refs/heads/") {
		return fmt.Errorf("pre-push: only branch updates are supported")
	}
	if ref[1] != head {
		return fmt.Errorf("pre-push: local object is not HEAD")
	}
	return nil
}

// readPrePushRef consumes the entire input before enforcing the single-ref policy.
func readPrePushRef(input io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(input)
	var refs [][]string
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 4 {
			return nil, fmt.Errorf("pre-push: malformed ref input")
		}
		refs = append(refs, fields)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("pre-push: reading refs: %w", err)
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("pre-push: no ref updates supplied")
	}
	if len(refs) != 1 {
		return nil, fmt.Errorf("pre-push: multiple ref updates are not supported")
	}
	return refs[0], nil
}
