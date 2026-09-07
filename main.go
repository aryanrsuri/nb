package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

const usage = `nb - nota bene

Usage: nb [-root DIR] [-json] COMMAND [ARGS]
Discover .nb in the current directory or its parents. Put flags before COMMAND.
Use -root DIR to start discovery elsewhere.

  init [DIR]             Initialize this directory (or DIR) as a notebook
  root                   Print the discovered notebook root
  index                  Map all existing notes and links (JSON)
  list [QUERY]           Find notes by case-insensitive path substring
  links [NOTE]           Show all links, or NOTE's outgoing links
  backlinks NOTE         Show links pointing to NOTE
  resolve SOURCE PATH    Print the absolute target of a source-relative link
  link SOURCE TARGET     Print [[relative/path]] for insertion into SOURCE
  open NOTE              Open an existing note in nvim (or NB_EDITOR)

NOTE, SOURCE and TARGET are root-relative or absolute; .md is optional.
PATH is source-relative, as written between [[ and ]]. Missing targets are
ignored. 'link' prints text only; it never edits or creates a file.
The local .nb/config.json sets the editor (default nvim); NB_EDITOR overrides it.
Editor values are executable names/paths, without arguments.
`

func run(args []string, out, stderr io.Writer) error {
	flags := flag.NewFlagSet("nb", flag.ContinueOnError)
	flags.SetOutput(stderr)
	start := flags.String("root", ".", "directory to discover the notebook from")
	asJSON := flags.Bool("json", false, "machine-readable output")
	flags.Usage = func() { fmt.Fprint(stderr, usage) }
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	args = flags.Args()
	if len(args) == 0 {
		_, err := fmt.Fprint(out, usage)
		return err
	}
	command, args := args[0], args[1:]
	minArgs, maxArgs := 0, 0
	switch command {
	case "index", "root":
	case "init", "list", "links":
		maxArgs = 1
	case "backlinks", "open":
		minArgs, maxArgs = 1, 1
	case "resolve", "link":
		minArgs, maxArgs = 2, 2
	default:
		return fmt.Errorf("unknown command %q; run nb -h", command)
	}
	if len(args) < minArgs || len(args) > maxArgs {
		return fmt.Errorf("wrong arguments for %s; run nb -h", command)
	}
	if command == "init" {
		if len(args) == 1 {
			if *start != "." {
				return fmt.Errorf("use either nb init DIR or nb -root DIR init")
			}
			*start = args[0]
		}
		root, err := initNotebook(*start)
		if err != nil {
			return err
		}
		return printResult(out, root, *asJSON)
	}
	root, err := notebookRoot(*start)
	if err != nil {
		return err
	}
	config, err := readConfig(root)
	if err != nil {
		return err
	}
	if command == "root" {
		return printResult(out, root, *asJSON)
	}
	n, err := scan(root, command == "index" || command == "links" || command == "backlinks")
	if err != nil {
		return err
	}
	var result any
	switch command {
	case "index":
		result = n
		*asJSON = true
	case "list":
		notes := []string{}
		query := ""
		if len(args) == 1 {
			query = strings.ToLower(args[0])
		}
		for _, note := range n.Notes {
			if strings.Contains(strings.ToLower(note), query) {
				notes = append(notes, note)
			}
		}
		result = notes
	case "links", "backlinks":
		links := []Link{}
		note := ""
		if len(args) > 0 {
			note, err = n.existing(args[0])
			if err != nil {
				return err
			}
		}
		for _, link := range n.Links {
			if note == "" || (command == "links" && link.Source == note) || (command == "backlinks" && link.Target == note) {
				links = append(links, link)
			}
		}
		result = links
	case "resolve":
		filename, err := n.resolve(args[0], args[1])
		if err != nil {
			return err
		}
		if filename != "" {
			result = filename
		}
	case "link":
		result, err = n.link(args[0], args[1])
		if err != nil {
			return err
		}
	case "open":
		note, err := n.existing(args[0])
		if err != nil {
			return err
		}
		editor := os.Getenv("NB_EDITOR")
		if editor == "" {
			editor = config.Editor
		}
		cmd := exec.Command(editor, "--", n.files[note])
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, out, stderr
		return cmd.Run()
	}
	return printResult(out, result, *asJSON)
}

func printResult(out io.Writer, result any, asJSON bool) error {
	if asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	var err error
	switch value := result.(type) {
	case string:
		_, err = fmt.Fprintln(out, value)
	case []string:
		for _, note := range value {
			if _, err = fmt.Fprintln(out, note); err != nil {
				return err
			}
		}
	case []Link:
		for _, link := range value {
			if _, err = fmt.Fprintf(out, "%s:%d:%d\t%s\n", link.Source, link.Line, link.Column, link.Target); err != nil {
				return err
			}
		}
	}
	return err
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "nb:", err)
		os.Exit(1)
	}
}
