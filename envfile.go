package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// loadStartupEnvironment loads an explicit file, or .env from the current
// directory when no file was specified. Existing process variables always win.
func loadStartupEnvironment(explicitPath string) (string, int, error) {
	path := strings.TrimSpace(explicitPath)
	required := path != ""
	if path == "" {
		path = ".env"
	}
	count, err := loadEnvFile(path)
	if err != nil {
		if !required && errors.Is(err, os.ErrNotExist) {
			return "", 0, nil
		}
		return "", 0, err
	}
	return path, count, nil
}

func loadEnvFile(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	loaded := 0
	scanner := bufio.NewScanner(file)
	// Environment files occasionally contain long certificates or tokens.
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		key, value, present, err := parseEnvLine(scanner.Text())
		if err != nil {
			return loaded, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
		if !present {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return loaded, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
		loaded++
	}
	if err := scanner.Err(); err != nil {
		return loaded, err
	}
	return loaded, nil
}

func parseEnvLine(line string) (string, string, bool, error) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	if strings.HasPrefix(line, "export ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	}
	key, raw, found := strings.Cut(line, "=")
	if !found {
		return "", "", false, errors.New("expected KEY=VALUE")
	}
	key = strings.TrimSpace(key)
	if !validEnvKey(key) {
		return "", "", false, fmt.Errorf("invalid variable name %q", key)
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return key, "", true, nil
	}
	if raw[0] == '\'' {
		end := strings.IndexByte(raw[1:], '\'')
		if end < 0 {
			return "", "", false, errors.New("unterminated single-quoted value")
		}
		end++
		if !validQuotedTail(raw[end+1:]) {
			return "", "", false, errors.New("unexpected text after quoted value")
		}
		return key, raw[1:end], true, nil
	}
	if raw[0] == '"' {
		end := -1
		escaped := false
		for i := 1; i < len(raw); i++ {
			if raw[i] == '"' && !escaped {
				end = i
				break
			}
			if raw[i] == '\\' && !escaped {
				escaped = true
			} else {
				escaped = false
			}
		}
		if end < 0 {
			return "", "", false, errors.New("unterminated double-quoted value")
		}
		if !validQuotedTail(raw[end+1:]) {
			return "", "", false, errors.New("unexpected text after quoted value")
		}
		value, err := strconv.Unquote(raw[:end+1])
		if err != nil {
			return "", "", false, fmt.Errorf("invalid quoted value: %w", err)
		}
		return key, value, true, nil
	}
	// Treat a hash preceded by whitespace as an inline comment. A hash inside a
	// value (for example a password) is preserved.
	for i := 0; i < len(raw); i++ {
		if raw[i] == '#' && i > 0 && (raw[i-1] == ' ' || raw[i-1] == '\t') {
			raw = strings.TrimSpace(raw[:i])
			break
		}
	}
	return key, raw, true, nil
}

func validQuotedTail(tail string) bool {
	tail = strings.TrimSpace(tail)
	return tail == "" || strings.HasPrefix(tail, "#")
}

func validEnvKey(key string) bool {
	if key == "" {
		return false
	}
	for i, r := range key {
		if !(r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
