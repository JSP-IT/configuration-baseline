package baseline

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Document struct {
	SchemaVersion   int
	AllowUnexpected bool
	Rules           []Rule
}

type Rule struct {
	Path   string
	Type   string
	Value  any
	Values []any
}

func LoadBaseline(path string) (Document, error) {
	value, err := loadDocument(path, "baseline")
	if err != nil {
		return Document{}, err
	}

	root, ok := value.(map[string]any)
	if !ok {
		return Document{}, errors.New("baseline must be a mapping")
	}
	if err := rejectUnknownFields(root, "schema_version", "allow_unexpected", "rules"); err != nil {
		return Document{}, fmt.Errorf("baseline: %w", err)
	}

	version, ok := root["schema_version"]
	if !ok {
		return Document{}, errors.New("baseline: schema_version is required")
	}
	schemaVersion, ok := integer(version)
	if !ok || schemaVersion != 1 {
		return Document{}, errors.New("baseline: schema_version must be 1")
	}

	document := Document{SchemaVersion: schemaVersion}
	if raw, ok := root["allow_unexpected"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return Document{}, errors.New("baseline: allow_unexpected must be a boolean")
		}
		document.AllowUnexpected = value
	}

	rawRules, ok := root["rules"]
	if !ok {
		return Document{}, errors.New("baseline: rules is required")
	}
	rules, ok := rawRules.([]any)
	if !ok {
		return Document{}, errors.New("baseline: rules must be a sequence")
	}

	seenPaths := make(map[string]struct{}, len(rules))
	document.Rules = make([]Rule, 0, len(rules))
	for i, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]any)
		if !ok {
			return Document{}, fmt.Errorf("baseline: rule %d must be a mapping", i+1)
		}
		rule, err := parseRule(ruleMap)
		if err != nil {
			return Document{}, fmt.Errorf("baseline: rule %d: %w", i+1, err)
		}
		if _, exists := seenPaths[rule.Path]; exists {
			return Document{}, fmt.Errorf("baseline: duplicate rule path %q", rule.Path)
		}
		seenPaths[rule.Path] = struct{}{}
		document.Rules = append(document.Rules, rule)
	}

	return document, nil
}

func LoadConfig(path string) (any, error) {
	return loadDocument(path, "configuration")
}

func loadDocument(path, kind string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s %q: could not be read; check that the path exists and is accessible", kind, path)
	}

	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		value, err := decodeJSON(data)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid JSON document", kind)
		}
		return value, nil
	case ".yaml", ".yml":
		value, err := decodeYAML(data)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid YAML document", kind)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("%s: unsupported file extension; use .json, .yaml, or .yml", kind)
	}
}

func decodeJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	value, err := decodeJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	return value, nil
}

func decodeJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}

	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}

	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("object key must be a string")
			}
			if _, exists := object[key]; exists {
				return nil, errors.New("duplicate object key")
			}
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		closing, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if closing != json.Delim('}') {
			return nil, errors.New("invalid JSON object")
		}
		return object, nil
	case '[':
		var array []any
		for decoder.More() {
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		closing, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		if closing != json.Delim(']') {
			return nil, errors.New("invalid JSON array")
		}
		return array, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}

func decodeYAML(data []byte) (any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var node yaml.Node
	if err := decoder.Decode(&node); err != nil {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("multiple YAML documents are not supported")
	}
	keepTimestampsAsText(&node)
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, err
	}
	return normalizeYAML(value)
}

// keepTimestampsAsText makes YAML timestamps compare as their literal text,
// so that an unquoted YAML date such as 2024-01-01 equals the JSON string
// "2024-01-01" instead of being reformatted as a time value.
func keepTimestampsAsText(node *yaml.Node) {
	if node.Kind == yaml.ScalarNode && node.ShortTag() == "!!timestamp" {
		node.Tag = "!!str"
	}
	for _, child := range node.Content {
		keepTimestampsAsText(child)
	}
}

func normalizeYAML(value any) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			normalized, err := normalizeYAML(child)
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case map[any]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			stringKey, ok := key.(string)
			if !ok {
				return nil, errors.New("mapping keys must be strings")
			}
			normalized, err := normalizeYAML(child)
			if err != nil {
				return nil, err
			}
			result[stringKey] = normalized
		}
		return result, nil
	case []any:
		result := make([]any, len(value))
		for i, child := range value {
			normalized, err := normalizeYAML(child)
			if err != nil {
				return nil, err
			}
			result[i] = normalized
		}
		return result, nil
	case float32:
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, errors.New("non-finite numbers are not supported")
		}
		return float64(value), nil
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("non-finite numbers are not supported")
		}
		return value, nil
	default:
		return value, nil
	}
}

func parseRule(fields map[string]any) (Rule, error) {
	if err := rejectUnknownFields(fields, "path", "type", "value", "values"); err != nil {
		return Rule{}, err
	}

	pathValue, ok := fields["path"]
	if !ok {
		return Rule{}, errors.New("path is required")
	}
	path, ok := pathValue.(string)
	if !ok {
		return Rule{}, errors.New("path must be a string")
	}
	if _, err := parsePointer(path); err != nil {
		return Rule{}, errors.New("path must be an RFC 6901 JSON Pointer")
	}

	typeValue, ok := fields["type"]
	if !ok {
		return Rule{}, errors.New("type is required")
	}
	ruleType, ok := typeValue.(string)
	if !ok {
		return Rule{}, errors.New("type must be a string")
	}

	value, hasValue := fields["value"]
	rawValues, hasValues := fields["values"]
	rule := Rule{Path: path, Type: ruleType, Value: value}

	switch ruleType {
	case "equals":
		if !hasValue || hasValues {
			return Rule{}, errors.New("equals rules require value and do not accept values")
		}
	case "exists", "absent":
		if hasValue || hasValues {
			return Rule{}, errors.New("exists and absent rules do not accept value or values")
		}
	case "one_of":
		if hasValue || !hasValues {
			return Rule{}, errors.New("one_of rules require values and do not accept value")
		}
		values, ok := rawValues.([]any)
		if !ok || len(values) == 0 {
			return Rule{}, errors.New("one_of values must be a non-empty sequence")
		}
		rule.Values = values
	default:
		return Rule{}, errors.New("unsupported rule type")
	}

	return rule, nil
}

func rejectUnknownFields(fields map[string]any, allowed ...string) error {
	set := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		set[field] = struct{}{}
	}
	unknown := make([]string, 0)
	for field := range fields {
		if _, ok := set[field]; !ok {
			unknown = append(unknown, field)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return fmt.Errorf("unsupported field %q", unknown[0])
	}
	return nil
}

func integer(value any) (int, bool) {
	switch value := value.(type) {
	case json.Number:
		n, err := strconv.Atoi(string(value))
		return n, err == nil
	case int:
		return value, true
	case int64:
		return int(value), int64(int(value)) == value
	case uint64:
		n := int(value)
		return n, uint64(n) == value
	default:
		return 0, false
	}
}
