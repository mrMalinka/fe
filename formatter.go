package main

import (
	"bytes"
	"fmt"
	"regexp"
	"text/template"
)

// namedFormatter formats strings with %(name)T syntax
// example: namedFormatter("hello %(name)s!", map[string]interface{}{"name": "world"})
func namedFormatter(format string, data map[string]interface{}) (string, []string, error) {
	// Extract all command tags
	cmdRegex := regexp.MustCompile(`#([a-z_]+)`)
	matches := cmdRegex.FindAllStringSubmatch(format, -1)

	commands := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			commands = append(commands, match[1])
		}
	}

	// remove all command tags from format string
	cleanedFormat := cmdRegex.ReplaceAllString(format, "")

	// process the main format string
	re := regexp.MustCompile(`%\((\w+)\)([a-zA-Z%])`)
	templateText := re.ReplaceAllString(cleanedFormat, "{{.${1}}}${2}")

	// handle literal percentages
	templateText = regexp.MustCompile(`%%`).ReplaceAllString(templateText, "%")

	// create template with custom function map
	tmpl, err := template.New("format").Funcs(template.FuncMap{
		"printf": fmt.Sprintf,
	}).Parse(templateText)
	if err != nil {
		return "", nil, err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", nil, err
	}

	return buf.String(), commands, nil
}

func (a *AppState) makeFormattingData() map[string]interface{} {
	return map[string]interface{}{
		"working_dir":  a.workingDir,
		"selected_dir": a.dirContents[a.selectedDir],
	}
}
