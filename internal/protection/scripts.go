package protection

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"mvdan.cc/sh/v3/syntax"
)

// maxSettingsBytes bounds how much of a hook settings file is read.
const maxSettingsBytes = 1 << 20

// envRef matches $NAME and ${NAME} in a hook command.
var envRef = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}|\$[A-Za-z_][A-Za-z0-9_]*`)

// projectDirVars are the variables harnesses set to the project root in
// hook commands.
var projectDirVars = []string{"CLAUDE_PROJECT_DIR", "GEMINI_PROJECT_DIR", "GEMINI_CWD"}

// addScripts protects the script files the evidence checks and the
// registered hooks run. Changing one changes what a check proves or what a
// hook enforces without touching any configuration file.
func (s *Set) addScripts(opts Options) {
	for _, command := range opts.EvidenceCommands {
		for _, path := range s.commandFiles(command, false) {
			s.addAbs(path, false, ReasonEvidenceScript)
		}
	}

	for _, settingsFile := range s.hookSettingsFiles(opts) {
		for _, command := range hookCommands(settingsFile) {
			for _, path := range s.commandFiles(command, false) {
				s.addAbs(path, false, ReasonHookScript)
			}
		}
	}
}

// hookSettingsFiles lists the JSON files that register hooks.
func (s *Set) hookSettingsFiles(opts Options) []string {
	codexHome := filepath.Join(s.home, ".codex")
	if value, ok := opts.LookupEnv("CODEX_HOME"); ok && filepath.IsAbs(value) {
		codexHome = value
	}

	base := []string{
		filepath.Join(s.home, ".claude", "settings.json"),
		filepath.Join(s.projectRoot, ".claude", "settings.json"),
		filepath.Join(s.projectRoot, ".claude", "settings.local.json"),
		filepath.Join(codexHome, "hooks.json"),
		filepath.Join(codexHome, "config.toml"),
		filepath.Join(s.projectRoot, ".codex", "hooks.json"),
		filepath.Join(s.projectRoot, ".codex", "config.toml"),
		filepath.Join(s.home, ".gemini", "settings.json"),
		filepath.Join(s.projectRoot, ".gemini", "settings.json"),
	}

	managed := claudeManagedPaths(opts.GOOS)
	files := make([]string, 0, len(base)+len(managed)+len(opts.HookFiles))
	files = append(files, base...)

	for _, dir := range managed {
		files = append(files, filepath.Join(dir, "managed-settings.json"))
	}

	files = append(files, codexManagedPaths(opts.GOOS, opts.LookupEnv)...)

	for _, file := range opts.HookFiles {
		files = append(files, s.absolute(file, s.projectRoot))
	}

	return files
}

// hookCommands returns every "command" string under the "hooks" key of a
// JSON settings file or a TOML config (Codex inline [hooks]).
func hookCommands(path string) []string {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSettingsBytes {
		return nil
	}

	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil
	}

	var root map[string]any

	if strings.EqualFold(filepath.Ext(path), ".toml") {
		err = toml.Unmarshal(data, &root)
	} else {
		err = json.Unmarshal(data, &root)
	}

	if err != nil {
		return nil
	}

	var commands []string

	collectCommands(root["hooks"], &commands)

	return commands
}

func collectCommands(node any, commands *[]string) {
	switch value := node.(type) {
	case map[string]any:
		for key, child := range value {
			if text, ok := child.(string); ok && key == "command" {
				*commands = append(*commands, text)

				continue
			}

			collectCommands(child, commands)
		}
	case []any:
		for _, child := range value {
			collectCommands(child, commands)
		}
	case []map[string]any:
		for _, child := range value {
			collectCommands(child, commands)
		}
	}
}

// commandFiles returns the files a command line names: words that resolve,
// against the project root, to a regular file, and for hooks also paths
// that do not exist yet, so a missing hook script cannot be created. Program
// names found only on PATH are left out. Words are read with shell syntax,
// so a quoted path keeps its spaces.
func (s *Set) commandFiles(command string, mustExist bool) []string {
	var files []string

	for _, word := range s.commandWords(command, 0) {
		if word == "" || strings.HasPrefix(word, "-") {
			continue
		}

		if !strings.Contains(word, "/") && !strings.Contains(word, ".") {
			continue
		}

		path := s.absolute(word, s.projectRoot)

		info, err := os.Stat(path)

		switch {
		case err == nil && info.Mode().IsRegular():
		case err != nil && !mustExist && strings.Contains(word, "/"):
		default:
			continue
		}

		files = append(files, path)
	}

	return files
}

// commandWords returns the words of command after quote removal and
// expansion of the variables the hook environment knows. A word holding
// whitespace that is not itself a file, such as the script of bash -c, is
// read again as a command. A command that does not parse is split on
// whitespace with quotes dropped, so it still yields every candidate.
func (s *Set) commandWords(command string, depth int) []string {
	file, err := syntax.NewParser(syntax.KeepComments(false)).Parse(
		strings.NewReader(command), "",
	)
	if err != nil {
		return strings.Fields(strings.NewReplacer(`"`, "", "'", "").Replace(s.expandRefs(command)))
	}

	var words []string

	syntax.Walk(file, func(node syntax.Node) bool {
		word, ok := node.(*syntax.Word)
		if !ok {
			return true
		}

		value, ok := s.wordValue(word)
		if !ok {
			return true
		}

		words = append(words, value)

		if depth < maxCommandDepth && strings.ContainsAny(value, " \t\n;&|") &&
			!s.isFile(value) {
			words = append(words, s.commandWords(value, depth+1)...)
		}

		return true
	})

	return words
}

// maxCommandDepth bounds how deep words are read again as commands.
const maxCommandDepth = 3

func (s *Set) isFile(word string) bool {
	info, err := os.Stat(s.absolute(word, s.projectRoot))

	return err == nil && info.Mode().IsRegular()
}

// wordValue joins the parts of word, reporting false for parts whose value
// is only known when the command runs (command substitution, arithmetic).
func (s *Set) wordValue(word *syntax.Word) (string, bool) {
	var b strings.Builder

	for _, part := range word.Parts {
		if !s.writePart(&b, part) {
			return "", false
		}
	}

	return b.String(), true
}

func (s *Set) writePart(b *strings.Builder, part syntax.WordPart) bool {
	switch p := part.(type) {
	case *syntax.Lit:
		b.WriteString(unescape(p.Value))
	case *syntax.SglQuoted:
		b.WriteString(p.Value)
	case *syntax.DblQuoted:
		for _, inner := range p.Parts {
			if !s.writePart(b, inner) {
				return false
			}
		}
	case *syntax.ParamExp:
		if p.Param == nil {
			return false
		}

		b.WriteString(s.expandRefs("${" + p.Param.Value + "}"))
	default:
		return false
	}

	return true
}

// expandRefs replaces the project directory variables with the project
// root and other $NAME references with their value, leaving unknown ones.
func (s *Set) expandRefs(command string) string {
	for _, name := range projectDirVars {
		command = strings.ReplaceAll(command, "${"+name+"}", s.projectRoot)
		command = strings.ReplaceAll(command, "$"+name, s.projectRoot)
	}

	return envRef.ReplaceAllStringFunc(command, func(ref string) string {
		name := strings.Trim(ref, "${}")
		if value, ok := s.lookupEnv(name); ok {
			return value
		}

		return "$" + name
	})
}

// unescape drops the backslashes of an unquoted word: policy\ check.sh
// names "policy check.sh".
func unescape(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}

	var b strings.Builder

	escaped := false

	for _, r := range value {
		if r == '\\' && !escaped {
			escaped = true

			continue
		}

		escaped = false

		b.WriteRune(r)
	}

	return b.String()
}
