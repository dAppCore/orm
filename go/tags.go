// SPDX-License-Identifier: EUPL-1.2

package orm

import (
	"dappco.re/go"
)

func storageFieldName(fallback, jsonTag string) (string, bool) {
	if jsonTag == "-" {
		return "", false
	}
	if jsonTag != "" {
		parts := core.SplitN(jsonTag, ",", 2)
		if parts[0] != "" {
			return parts[0], true
		}
	}
	return fallback, true
}

func rowValueForField(row map[string]any, names []string) (any, string, bool) {
	for _, name := range names {
		if val, exists := row[name]; exists {
			return val, name, true
		}
	}
	for key, val := range row {
		for _, name := range names {
			if normalizeName(key) == normalizeName(name) {
				return val, key, true
			}
		}
	}
	return nil, "", false
}

func fieldNames(goName, jsonTag string) ([]string, bool) {
	storageName, ok := storageFieldName(goName, jsonTag)
	if !ok {
		return nil, false
	}
	if storageName == goName {
		return []string{goName}, true
	}
	return []string{storageName, goName}, true
}

func findSchemaFieldForNames(schema Schema, names []string) Field {
	for _, name := range names {
		if schemaField := findSchemaField(schema, name); schemaField.Name != "" {
			return schemaField
		}
	}
	return Field{}
}
