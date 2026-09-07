package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

type Link struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

type Notebook struct {
	Root  string   `json:"root"`
	Notes []string `json:"notes"`
	Links []Link   `json:"links"`
	files map[string]string
}

// Paths in notes use forward slashes, independent of the host OS.
func notePath(name string) (string, error) {
	name = filepath.ToSlash(name)
	if name == "" || strings.ContainsAny(name, "\r\n\t[]") || path.IsAbs(name) || filepath.IsAbs(name) {
		return "", fmt.Errorf("invalid note path %q", name)
	}
	name = path.Clean(name)
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return "", fmt.Errorf("note path leaves the notebook: %q", name)
	}
	return name, nil
}

func (n *Notebook) note(name string) (string, error) {
	if filepath.IsAbs(name) {
		// Canonicalize parent directories just as root discovery does. Keep the
		// final component intact so symlink note files remain excluded.
		parent, suffix := filepath.Dir(name), filepath.Base(name)
		for {
			real, err := filepath.EvalSymlinks(parent)
			if err == nil {
				name = filepath.Join(real, suffix)
				break
			}
			if !os.IsNotExist(err) || filepath.Dir(parent) == parent {
				return "", err
			}
			// A link may target a note whose parent directory is not created yet.
			suffix = filepath.Join(filepath.Base(parent), suffix)
			parent = filepath.Dir(parent)
		}
		var err error
		name, err = filepath.Rel(n.Root, name)
		if err != nil {
			return "", err
		}
	} else if id, err := notePath(name); err == nil {
		// An exact extensionless ID takes precedence over optional .md syntax.
		if _, exists := n.files[id]; exists {
			return id, nil
		}
	}
	return notePath(strings.TrimSuffix(name, ".md"))
}

func (n *Notebook) existing(name string) (string, error) {
	name, err := n.note(name)
	if err != nil {
		return "", err
	}
	if _, ok := n.files[name]; !ok {
		return "", fmt.Errorf("note does not exist: %s", name)
	}
	return name, nil
}

func relativeTarget(source, target string) (string, error) {
	if target == "" || path.IsAbs(target) || filepath.IsAbs(target) {
		return "", fmt.Errorf("link must be relative to its source")
	}
	return notePath(path.Join(path.Dir(source), target))
}

var wikiLink = regexp.MustCompile(`\[\[([^\[\]\r\n]+)\]\]`)

func scan(root string, withLinks bool) (*Notebook, error) {
	root, err := directory(root)
	if err != nil {
		return nil, err
	}
	n := &Notebook{Root: root, Notes: []string{}, Links: []Link{}, files: map[string]string{}}
	err = filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == ".nb" {
				return filepath.SkipDir
			}
			// Nested notebooks own their notes and are indexed separately.
			if filename != root {
				if _, err := os.Stat(filepath.Join(filename, ".nb")); err == nil {
					return filepath.SkipDir
				} else if !os.IsNotExist(err) {
					return err
				}
			}
			return nil
		}
		// Ignore symlinks so a notebook cannot accidentally include external files.
		if !entry.Type().IsRegular() || filepath.Ext(filename) != ".md" {
			return nil
		}
		rel, err := filepath.Rel(root, filename)
		if err != nil {
			return err
		}
		name, err := notePath(strings.TrimSuffix(rel, ".md"))
		if err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		n.Notes = append(n.Notes, name)
		n.files[name] = filename
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !withLinks {
		return n, nil
	}
	for _, source := range n.Notes {
		data, err := os.ReadFile(n.files[source])
		if err != nil {
			return nil, err
		}
		for lineIndex, line := range strings.Split(string(data), "\n") {
			for _, match := range wikiLink.FindAllStringSubmatchIndex(line, -1) {
				target, err := relativeTarget(source, line[match[2]:match[3]])
				if err != nil {
					continue
				}
				if _, exists := n.files[target]; !exists {
					continue
				}
				n.Links = append(n.Links, Link{source, target, lineIndex + 1, match[0] + 1})
			}
		}
	}
	return n, nil
}

func (n *Notebook) resolve(source, target string) (string, error) {
	source, err := n.existing(source)
	if err != nil {
		return "", err
	}
	target, err = relativeTarget(source, target)
	if err != nil {
		return "", nil
	}
	return n.files[target], nil
}

func (n *Notebook) link(source, target string) (string, error) {
	source, err := n.existing(source)
	if err != nil {
		return "", err
	}
	target, err = n.note(target)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(filepath.FromSlash(path.Dir(source)), filepath.FromSlash(target))
	if err != nil {
		return "", err
	}
	return "[[" + filepath.ToSlash(rel) + "]]", nil
}
