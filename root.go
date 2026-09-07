package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Config struct {
	Editor string `json:"editor"`
}

func directory(name string) (string, error) {
	name, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	name, err = filepath.EvalSymlinks(name)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(name)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", name)
	}
	return name, nil
}

// The directory containing .nb is the root; no absolute path is stored.
func notebookRoot(start string) (string, error) {
	current, err := directory(start)
	if err != nil {
		return "", err
	}
	for {
		marker := filepath.Join(current, ".nb")
		info, err := os.Stat(marker)
		if err == nil {
			if !info.IsDir() {
				return "", fmt.Errorf("not a directory: %s", marker)
			}
			return current, nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("no notebook found from %s; run nb init in your notes folder", start)
		}
		current = parent
	}
}

func readConfig(root string) (Config, error) {
	filename := filepath.Join(root, ".nb", "config.json")
	file, err := os.Open(filename)
	if err != nil {
		return Config{}, err
	}
	defer file.Close()
	config := &Config{Editor: "nvim"}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("%s: %w", filename, err)
	}
	if config == nil || config.Editor == "" {
		return Config{}, fmt.Errorf("%s: expected an object with a nonempty editor executable", filename)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Config{}, fmt.Errorf("%s: expected a single JSON object", filename)
	}
	return *config, nil
}

func initNotebook(name string) (string, error) {
	root, err := directory(name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(root, ".nb"), 0755); err != nil {
		return "", err
	}
	filename := filepath.Join(root, ".nb", "config.json")
	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if os.IsExist(err) {
		_, err = readConfig(root)
		return root, err
	}
	if err != nil {
		return "", err
	}
	writeErr := json.NewEncoder(file).Encode(Config{Editor: "nvim"})
	closeErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	return root, closeErr
}
