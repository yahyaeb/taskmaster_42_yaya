package helpers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CopyFile copies the file at src to dst, creating or overwriting dst.
func CopyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// UpdateProgramBlock reads the YAML config at path, finds the block for program,
// passes its lines to mutate, and writes the result back.
func UpdateProgramBlock(path, program string, mutate func([]string) ([]string, error)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")

	start := -1
	for i, line := range lines {
		if line == "  "+program+":" {
			start = i
			break
		}
	}
	if start < 0 {
		return fmt.Errorf("program %q not found in %s", program, path)
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") &&
			strings.HasSuffix(strings.TrimSpace(line), ":") {
			end = i
			break
		}
	}

	block := append([]string(nil), lines[start+1:end]...)
	block, err = mutate(block)
	if err != nil {
		return err
	}

	out := append([]string{}, lines[:start+1]...)
	out = append(out, block...)
	out = append(out, lines[end:]...)
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0644)
}

// ReplaceBlockLines swaps the lines starting with key inside a YAML block.
func ReplaceBlockLines(block []string, key string, replacement []string) ([]string, error) {
	prefix := "    " + key + ":"
	for i, line := range block {
		if strings.HasPrefix(line, prefix) {
			out := append([]string{}, block[:i]...)
			out = append(out, replacement...)
			out = append(out, block[i+1:]...)
			return out, nil
		}
	}
	return nil, fmt.Errorf("key %q not found in block", key)
}

// RunCmd runs an external command in root and returns a wrapped error with stdout+stderr.
func RunCmd(root, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, buf.String())
	}
	return nil
}

// Must panics if err is non-nil. Use only in test/eval code, never in production paths.
func Must(err error) {
	if err != nil {
		panic(err)
	}
}
