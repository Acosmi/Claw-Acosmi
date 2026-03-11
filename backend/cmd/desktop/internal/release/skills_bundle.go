package release

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LoadSkillsPackageManifest reads a desktop skill packaging whitelist.
func LoadSkillsPackageManifest(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open manifest: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	entries := make([]string, 0, 40)
	seen := make(map[string]struct{})
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		entry, err := normalizeSkillsPackageEntry(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNo, err)
		}
		if _, ok := seen[entry]; ok {
			return nil, fmt.Errorf("%s:%d: duplicate entry %q", path, lineNo, entry)
		}
		seen[entry] = struct{}{}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("manifest %s contains no skill entries", path)
	}
	return entries, nil
}

// StageSkillsPackage copies only whitelisted skill directories into dstRoot.
func StageSkillsPackage(srcRoot, dstRoot, manifestPath string) error {
	entries, err := LoadSkillsPackageManifest(manifestPath)
	if err != nil {
		return err
	}
	return StageSkillsPackageEntries(srcRoot, dstRoot, entries)
}

// StageSkillsPackageEntries copies the given skill directories into dstRoot.
func StageSkillsPackageEntries(srcRoot, dstRoot string, entries []string) error {
	srcRoot = filepath.Clean(srcRoot)
	dstRoot = filepath.Clean(dstRoot)
	if strings.TrimSpace(srcRoot) == "" {
		return fmt.Errorf("source root must not be empty")
	}
	if strings.TrimSpace(dstRoot) == "" {
		return fmt.Errorf("destination root must not be empty")
	}
	if len(entries) == 0 {
		return fmt.Errorf("no skill entries provided")
	}

	for _, entry := range entries {
		if err := stageSkillsPackageEntry(srcRoot, dstRoot, entry); err != nil {
			return err
		}
	}
	return nil
}

func normalizeSkillsPackageEntry(raw string) (string, error) {
	entry := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
	switch {
	case entry == "", entry == ".":
		return "", fmt.Errorf("empty skill entry")
	case filepath.IsAbs(raw):
		return "", fmt.Errorf("absolute paths are not allowed: %q", raw)
	case entry == "_archived", strings.HasPrefix(entry, "_archived/"):
		return "", fmt.Errorf("archived skills are not allowed: %q", raw)
	case entry == "..", strings.HasPrefix(entry, "../"), strings.Contains(entry, "/../"):
		return "", fmt.Errorf("path traversal is not allowed: %q", raw)
	}
	return entry, nil
}

func stageSkillsPackageEntry(srcRoot, dstRoot, entry string) error {
	srcDir := filepath.Join(srcRoot, filepath.FromSlash(entry))
	info, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("stat %s: %w", entry, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("skill entry %s is not a directory", entry)
	}
	skillFile := filepath.Join(srcDir, "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		return fmt.Errorf("skill entry %s is missing SKILL.md", entry)
	}

	dstDir := filepath.Join(dstRoot, filepath.FromSlash(entry))
	return copyTree(srcDir, dstDir)
}

func copyTree(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported in packaged skills: %s", path)
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := dstDir
		if rel != "." {
			target = filepath.Join(dstDir, rel)
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(srcPath, dstPath string, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Close()
}
