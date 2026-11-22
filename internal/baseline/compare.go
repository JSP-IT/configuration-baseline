package baseline

import (
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"reflect"
	"sort"
	"strconv"
	"strings"
)

var errorsForPointer = errors.New("invalid JSON Pointer")

type Finding struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

type Summary struct {
	Missing    int `json:"missing"`
	Unexpected int `json:"unexpected"`
	Modified   int `json:"modified"`
	Total      int `json:"total"`
}

type Report struct {
	Findings []Finding `json:"findings"`
	Summary  Summary   `json:"summary"`
}

type resultKey struct {
	path   string
	status string
}

func Compare(document Document, config any) Report {
	found := make(map[resultKey]struct{})
	rulePaths := make([][]string, 0, len(document.Rules))

	for _, rule := range document.Rules {
		segments, _ := parsePointer(rule.Path)
		rulePaths = append(rulePaths, segments)
		actual, exists := lookup(config, segments)

		switch rule.Type {
		case "equals":
			if !exists {
				addFinding(found, rule.Path, "MISSING")
				continue
			}
			compareValues(rule.Path, rule.Value, actual, found)
		case "exists":
			if !exists {
				addFinding(found, rule.Path, "MISSING")
			}
		case "absent":
			if exists {
				addFinding(found, rule.Path, "UNEXPECTED")
			}
		case "one_of":
			if !exists {
				addFinding(found, rule.Path, "MISSING")
				continue
			}
			matches := false
			for _, allowed := range rule.Values {
				if equalValues(allowed, actual) {
					matches = true
					break
				}
			}
			if !matches {
				addFinding(found, rule.Path, "MODIFIED")
			}
		}
	}

	if !document.AllowUnexpected {
		for _, path := range leafPaths(config, nil) {
			covered := false
			for _, rulePath := range rulePaths {
				if hasPrefix(path, rulePath) {
					covered = true
					break
				}
			}
			if !covered {
				addFinding(found, encodePointer(path), "UNEXPECTED")
			}
		}
	}

	report := Report{Findings: make([]Finding, 0, len(found))}
	for finding := range found {
		report.Findings = append(report.Findings, Finding{Path: finding.path, Status: finding.status})
		switch finding.status {
		case "MISSING":
			report.Summary.Missing++
		case "UNEXPECTED":
			report.Summary.Unexpected++
		case "MODIFIED":
			report.Summary.Modified++
		}
	}
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Path == report.Findings[j].Path {
			return report.Findings[i].Status < report.Findings[j].Status
		}
		return report.Findings[i].Path < report.Findings[j].Path
	})
	report.Summary.Total = len(report.Findings)
	return report
}

func compareValues(path string, expected, actual any, found map[resultKey]struct{}) {
	expectedMap, expectedIsMap := expected.(map[string]any)
	actualMap, actualIsMap := actual.(map[string]any)
	if expectedIsMap || actualIsMap {
		if !expectedIsMap || !actualIsMap {
			addFinding(found, path, "MODIFIED")
			return
		}
		keys := make([]string, 0, len(expectedMap)+len(actualMap))
		keySet := make(map[string]struct{}, len(expectedMap)+len(actualMap))
		for key := range expectedMap {
			keySet[key] = struct{}{}
			keys = append(keys, key)
		}
		for key := range actualMap {
			if _, exists := keySet[key]; !exists {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			expectedValue, inExpected := expectedMap[key]
			actualValue, inActual := actualMap[key]
			childPath := appendPointer(path, key)
			switch {
			case !inActual:
				addFinding(found, childPath, "MISSING")
			case !inExpected:
				addFinding(found, childPath, "UNEXPECTED")
			default:
				compareValues(childPath, expectedValue, actualValue, found)
			}
		}
		return
	}

	expectedArray, expectedIsArray := expected.([]any)
	actualArray, actualIsArray := actual.([]any)
	if expectedIsArray || actualIsArray {
		if !expectedIsArray || !actualIsArray {
			addFinding(found, path, "MODIFIED")
			return
		}
		maxLength := len(expectedArray)
		if len(actualArray) > maxLength {
			maxLength = len(actualArray)
		}
		for i := 0; i < maxLength; i++ {
			childPath := appendPointer(path, strconv.Itoa(i))
			switch {
			case i >= len(actualArray):
				addFinding(found, childPath, "MISSING")
			case i >= len(expectedArray):
				addFinding(found, childPath, "UNEXPECTED")
			default:
				compareValues(childPath, expectedArray[i], actualArray[i], found)
			}
		}
		return
	}

	if !equalValues(expected, actual) {
		addFinding(found, path, "MODIFIED")
	}
}

func equalValues(left, right any) bool {
	leftNumber, leftIsNumber := numberRat(left)
	rightNumber, rightIsNumber := numberRat(right)
	if leftIsNumber || rightIsNumber {
		return leftIsNumber && rightIsNumber && leftNumber.Cmp(rightNumber) == 0
	}

	leftMap, leftIsMap := left.(map[string]any)
	rightMap, rightIsMap := right.(map[string]any)
	if leftIsMap || rightIsMap {
		if !leftIsMap || !rightIsMap || len(leftMap) != len(rightMap) {
			return false
		}
		for key, value := range leftMap {
			other, exists := rightMap[key]
			if !exists || !equalValues(value, other) {
				return false
			}
		}
		return true
	}

	leftArray, leftIsArray := left.([]any)
	rightArray, rightIsArray := right.([]any)
	if leftIsArray || rightIsArray {
		if !leftIsArray || !rightIsArray || len(leftArray) != len(rightArray) {
			return false
		}
		for i := range leftArray {
			if !equalValues(leftArray[i], rightArray[i]) {
				return false
			}
		}
		return true
	}

	return reflect.DeepEqual(left, right)
}

func numberRat(value any) (*big.Rat, bool) {
	switch value := value.(type) {
	case json.Number:
		if rat, ok := new(big.Rat).SetString(string(value)); ok {
			return rat, true
		}
		number, _, err := big.ParseFloat(string(value), 10, 256, big.ToNearestEven)
		if err != nil {
			return nil, false
		}
		rat, _ := number.Rat(nil)
		return rat, true
	case int:
		return new(big.Rat).SetInt64(int64(value)), true
	case int8:
		return new(big.Rat).SetInt64(int64(value)), true
	case int16:
		return new(big.Rat).SetInt64(int64(value)), true
	case int32:
		return new(big.Rat).SetInt64(int64(value)), true
	case int64:
		return new(big.Rat).SetInt64(value), true
	case uint:
		return new(big.Rat).SetUint64(uint64(value)), true
	case uint8:
		return new(big.Rat).SetUint64(uint64(value)), true
	case uint16:
		return new(big.Rat).SetUint64(uint64(value)), true
	case uint32:
		return new(big.Rat).SetUint64(uint64(value)), true
	case uint64:
		return new(big.Rat).SetUint64(value), true
	case float32:
		number := float64(value)
		if math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, false
		}
		rat, ok := new(big.Rat).SetString(strconv.FormatFloat(number, 'g', -1, 32))
		return rat, ok
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, false
		}
		rat, ok := new(big.Rat).SetString(strconv.FormatFloat(value, 'g', -1, 64))
		return rat, ok
	default:
		return nil, false
	}
}

func lookup(value any, segments []string) (any, bool) {
	for _, segment := range segments {
		switch current := value.(type) {
		case map[string]any:
			next, exists := current[segment]
			if !exists {
				return nil, false
			}
			value = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || strconv.Itoa(index) != segment || index >= len(current) {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func leafPaths(value any, path []string) [][]string {
	switch value := value.(type) {
	case map[string]any:
		if len(value) == 0 {
			return [][]string{append([]string(nil), path...)}
		}
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var paths [][]string
		for _, key := range keys {
			paths = append(paths, leafPaths(value[key], append(append([]string(nil), path...), key))...)
		}
		return paths
	case []any:
		if len(value) == 0 {
			return [][]string{append([]string(nil), path...)}
		}
		var paths [][]string
		for index, child := range value {
			paths = append(paths, leafPaths(child, append(append([]string(nil), path...), strconv.Itoa(index)))...)
		}
		return paths
	default:
		return [][]string{append([]string(nil), path...)}
	}
}

func hasPrefix(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

func addFinding(found map[resultKey]struct{}, path, status string) {
	found[resultKey{path: path, status: status}] = struct{}{}
}

func parsePointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, errorsForPointer
	}
	rawSegments := strings.Split(pointer[1:], "/")
	segments := make([]string, len(rawSegments))
	for i, segment := range rawSegments {
		var builder strings.Builder
		for j := 0; j < len(segment); j++ {
			if segment[j] != '~' {
				builder.WriteByte(segment[j])
				continue
			}
			if j+1 >= len(segment) {
				return nil, errorsForPointer
			}
			j++
			switch segment[j] {
			case '0':
				builder.WriteByte('~')
			case '1':
				builder.WriteByte('/')
			default:
				return nil, errorsForPointer
			}
		}
		segments[i] = builder.String()
	}
	return segments, nil
}

func encodePointer(segments []string) string {
	var builder strings.Builder
	for _, segment := range segments {
		builder.WriteByte('/')
		segment = strings.ReplaceAll(segment, "~", "~0")
		segment = strings.ReplaceAll(segment, "/", "~1")
		builder.WriteString(segment)
	}
	return builder.String()
}

func appendPointer(pointer, segment string) string {
	segment = strings.ReplaceAll(segment, "~", "~0")
	segment = strings.ReplaceAll(segment, "/", "~1")
	return pointer + "/" + segment
}
