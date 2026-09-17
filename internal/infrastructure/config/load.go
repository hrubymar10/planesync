// Package config loads configuration from disk and resolves environment values.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/hrubymar10/planesync/internal/domain/configuration"
)

const (
	jiraTokenEnvironment  = "JIRA_TOKEN"
	planeTokenEnvironment = "PLANE_TOKEN"
	defaultThrottleMS     = 200
)

// Load reads, expands, validates, and returns configuration from path.
func Load(path string) (configuration.Config, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return configuration.Config{}, fmt.Errorf("read configuration: %w", err)
	}

	normalized, err := normalizeJSONC(contents)
	if err != nil {
		return configuration.Config{}, fmt.Errorf("normalize configuration: %w", err)
	}

	var document any
	if err := json.Unmarshal(normalized, &document); err != nil {
		return configuration.Config{}, fmt.Errorf("parse configuration: %w", err)
	}
	document = expandEnvironment(document)

	expanded, err := json.Marshal(document)
	if err != nil {
		return configuration.Config{}, fmt.Errorf("prepare configuration: %w", err)
	}

	result := configuration.Config{Defaults: configuration.Defaults{ThrottleMS: defaultThrottleMS}}
	if err := json.Unmarshal(expanded, &result); err != nil {
		return configuration.Config{}, fmt.Errorf("decode configuration: %w", err)
	}
	if strings.TrimSpace(result.Jira.AuthType) == "" {
		result.Jira.AuthType = "basic"
	}
	if err := resolveTokens(&result); err != nil {
		return configuration.Config{}, err
	}
	if err := result.Validate(); err != nil {
		return configuration.Config{}, err
	}
	return result, nil
}

func expandEnvironment(value any) any {
	switch value := value.(type) {
	case string:
		return os.ExpandEnv(value)
	case []any:
		for i := range value {
			value[i] = expandEnvironment(value[i])
		}
	case map[string]any:
		for key := range value {
			value[key] = expandEnvironment(value[key])
		}
	}
	return value
}

func resolveTokens(result *configuration.Config) error {
	jiraDefault := firstNonEmpty(result.Jira.Token.Reveal(), os.Getenv(jiraTokenEnvironment))
	planeDefault := firstNonEmpty(result.Plane.Token.Reveal(), os.Getenv(planeTokenEnvironment))

	for i := range result.Projects {
		project := &result.Projects[i]
		jiraToken := firstNonEmpty(project.JiraToken.Reveal(), jiraDefault)
		if jiraToken == "" {
			return fmt.Errorf("projects[%d].jira_token is required: set a project override, jira.token, or %s", i, jiraTokenEnvironment)
		}
		planeToken := firstNonEmpty(project.PlaneToken.Reveal(), planeDefault)
		if planeToken == "" {
			return fmt.Errorf("projects[%d].plane_token is required: set a project override, plane.token, or %s", i, planeTokenEnvironment)
		}
		project.JiraToken = configuration.Secret(jiraToken)
		project.PlaneToken = configuration.Secret(planeToken)
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeJSONC(input []byte) ([]byte, error) {
	withoutComments, err := stripComments(input)
	if err != nil {
		return nil, err
	}
	return stripTrailingCommas(withoutComments), nil
}

func stripComments(input []byte) ([]byte, error) {
	output := make([]byte, 0, len(input))
	inString := false
	escaped := false

	for i := 0; i < len(input); i++ {
		current := input[i]
		if inString {
			output = append(output, current)
			switch {
			case escaped:
				escaped = false
			case current == '\\':
				escaped = true
			case current == '"':
				inString = false
			}
			continue
		}

		if current == '"' {
			inString = true
			output = append(output, current)
			continue
		}
		if current != '/' || i+1 >= len(input) {
			output = append(output, current)
			continue
		}

		switch input[i+1] {
		case '/':
			i += 2
			for i < len(input) && input[i] != '\n' {
				i++
			}
			if i < len(input) {
				output = append(output, '\n')
			}
		case '*':
			i += 2
			closed := false
			for i < len(input) {
				if input[i] == '\n' {
					output = append(output, '\n')
				}
				if input[i] == '*' && i+1 < len(input) && input[i+1] == '/' {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, fmt.Errorf("unterminated block comment")
			}
		default:
			output = append(output, current)
		}
	}
	return output, nil
}

func stripTrailingCommas(input []byte) []byte {
	output := make([]byte, 0, len(input))
	inString := false
	escaped := false

	for i, current := range input {
		if inString {
			output = append(output, current)
			switch {
			case escaped:
				escaped = false
			case current == '\\':
				escaped = true
			case current == '"':
				inString = false
			}
			continue
		}
		if current == '"' {
			inString = true
			output = append(output, current)
			continue
		}
		if current == ',' {
			next := i + 1
			for next < len(input) && isJSONWhitespace(input[next]) {
				next++
			}
			if next < len(input) && (input[next] == '}' || input[next] == ']') {
				continue
			}
		}
		output = append(output, current)
	}
	return output
}

func isJSONWhitespace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}
