package antigravity

import (
	"encoding/json"
	"fmt"
	"strings"
)

// CleanJSONSchema recursively cleans JSON Schema to conform to Gemini API requirements
// (exactly like Antigravity-Manager's clean_json_schema)
//
// 1. Flatten $ref: Replace references with actual definitions
// 2. Remove unsupported fields: $schema, additionalProperties, format, default, etc.
// 3. Handle union types: ["string", "null"] -> "string"
// 4. Handle anyOf/oneOf: Extract first non-null type
// 5. Convert type field values to lowercase (Gemini v1internal requirement)
// 6. Remove numeric validation fields: multipleOf, exclusiveMinimum, etc.
// 7. Migrate constraints to description: [Constraint: minLen: 1]
// 8. Ensure enum values are strings
// 9. Validate required fields exist in properties
func CleanJSONSchema(schema map[string]interface{}) {
	// 0. Pre-process: Flatten $ref (Schema Flattening)
	defs := make(map[string]interface{})

	// Extract $defs or definitions
	if d, ok := schema["$defs"].(map[string]interface{}); ok {
		for k, v := range d {
			defs[k] = v
		}
		delete(schema, "$defs")
	}
	if d, ok := schema["definitions"].(map[string]interface{}); ok {
		for k, v := range d {
			defs[k] = v
		}
		delete(schema, "definitions")
	}

	if len(defs) > 0 {
		flattenRefs(schema, defs)
	}

	// Recursive clean
	cleanJSONSchemaRecursive(schema)
}

// flattenRefs recursively expands $ref references
func flattenRefs(schema map[string]interface{}, defs map[string]interface{}) {
	// Check and replace $ref
	if refPath, ok := schema["$ref"].(string); ok {
		delete(schema, "$ref")

		// Parse reference name (e.g., #/$defs/MyType -> MyType)
		parts := strings.Split(refPath, "/")
		refName := parts[len(parts)-1]

		if defSchema, ok := defs[refName]; ok {
			if defMap, ok := defSchema.(map[string]interface{}); ok {
				// Merge definition content into current schema
				for k, v := range defMap {
					// Only insert if key doesn't exist (avoid override)
					if _, exists := schema[k]; !exists {
						// Deep copy to avoid shared references
						schema[k] = deepCopyValue(v)
					}
				}
				// Recursively process refs in merged content
				flattenRefs(schema, defs)
			}
		}
	}

	// Traverse child nodes
	for _, v := range schema {
		if childMap, ok := v.(map[string]interface{}); ok {
			flattenRefs(childMap, defs)
		} else if arr, ok := v.([]interface{}); ok {
			for _, item := range arr {
				if itemMap, ok := item.(map[string]interface{}); ok {
					flattenRefs(itemMap, defs)
				}
			}
		}
	}
}

// deepCopyValue creates a deep copy of a value
func deepCopyValue(v interface{}) interface{} {
	switch val := v.(type) {
	case map[string]interface{}:
		copied := make(map[string]interface{})
		for k, v := range val {
			copied[k] = deepCopyValue(v)
		}
		return copied
	case []interface{}:
		copied := make([]interface{}, len(val))
		for i, v := range val {
			copied[i] = deepCopyValue(v)
		}
		return copied
	default:
		return v
	}
}

// cleanJSONSchemaRecursive recursively cleans the schema
func cleanJSONSchemaRecursive(value interface{}) {
	schema, ok := value.(map[string]interface{})
	if !ok {
		if arr, ok := value.([]interface{}); ok {
			for _, v := range arr {
				cleanJSONSchemaRecursive(v)
			}
		}
		return
	}

	// 1. [CRITICAL] Deep recursive processing: must traverse all field values
	for _, v := range schema {
		cleanJSONSchemaRecursive(v)
	}

	// 2. Collect and process validation fields (migrate constraints to description)
	var constraints []string

	// Validation fields to migrate
	validationFields := []struct {
		field string
		label string
	}{
		{"pattern", "pattern"},
		{"minLength", "minLen"},
		{"maxLength", "maxLen"},
		{"minimum", "min"},
		{"maximum", "max"},
		{"minItems", "minItems"},
		{"maxItems", "maxItems"},
		{"exclusiveMinimum", "exclMin"},
		{"exclusiveMaximum", "exclMax"},
		{"multipleOf", "multipleOf"},
		{"format", "format"},
	}

	for _, vf := range validationFields {
		if val, exists := schema[vf.field]; exists {
			// Only migrate if value is simple type
			switch v := val.(type) {
			case string:
				constraints = append(constraints, fmt.Sprintf("%s: %s", vf.label, v))
				delete(schema, vf.field)
			case float64:
				constraints = append(constraints, fmt.Sprintf("%s: %v", vf.label, v))
				delete(schema, vf.field)
			case int:
				constraints = append(constraints, fmt.Sprintf("%s: %d", vf.label, v))
				delete(schema, vf.field)
			case bool:
				constraints = append(constraints, fmt.Sprintf("%s: %v", vf.label, v))
				delete(schema, vf.field)
			default:
				// [CRITICAL FIX] If not simple type (e.g., Object), it might be a property named "pattern"
				// Must keep it, otherwise we'd delete a property!
			}
		}
	}

	// 3. Append constraint info to description
	if len(constraints) > 0 {
		suffix := fmt.Sprintf(" [Constraint: %s]", strings.Join(constraints, ", "))
		if desc, ok := schema["description"].(string); ok {
			schema["description"] = desc + suffix
		} else {
			schema["description"] = suffix
		}
	}

	// 4. [NEW FIX] Handle anyOf/oneOf union types - extract type before removal
	// FastMCP and other tools generate anyOf: [{"type": "string"}, {"type": "null"}] for Optional types
	// Gemini doesn't support anyOf, but we need to preserve type information
	if _, hasType := schema["type"]; !hasType {
		// Try to extract from anyOf
		if anyOf, ok := schema["anyOf"].([]interface{}); ok {
			if extractedType := extractTypeFromUnion(anyOf); extractedType != "" {
				schema["type"] = extractedType
			}
		}
		// If anyOf didn't extract, try oneOf
		if _, hasType := schema["type"]; !hasType {
			if oneOf, ok := schema["oneOf"].([]interface{}); ok {
				if extractedType := extractTypeFromUnion(oneOf); extractedType != "" {
					schema["type"] = extractedType
				}
			}
		}
	}

	// 5. Hard remove interfering fields (Hard Blacklist)
	hardRemoveFields := []string{
		"$schema",
		"$id",
		"additionalProperties",
		"enumCaseInsensitive",
		"enumNormalizeWhitespace",
		"uniqueItems",
		"default",
		"const",
		"examples",
		"propertyNames",
		"anyOf",
		"oneOf",
		"allOf",
		"not",
		"if",
		"then",
		"else",
		"dependencies",
		"dependentSchemas",
		"dependentRequired",
		"cache_control",
		"contentEncoding",
		"contentMediaType",
		"deprecated",
		"readOnly",
		"writeOnly",
	}
	for _, field := range hardRemoveFields {
		delete(schema, field)
	}

	// 6. [NEW FIX] Ensure required fields exist in properties
	// Gemini strictly validates: required fields not in properties cause INVALID_ARGUMENT
	if properties, ok := schema["properties"].(map[string]interface{}); ok {
		validPropKeys := make(map[string]bool)
		for k := range properties {
			validPropKeys[k] = true
		}

		if required, ok := schema["required"].([]interface{}); ok {
			var filteredRequired []interface{}
			for _, r := range required {
				if rStr, ok := r.(string); ok {
					if validPropKeys[rStr] {
						filteredRequired = append(filteredRequired, r)
					}
				}
			}
			if len(filteredRequired) != len(required) {
				schema["required"] = filteredRequired
			}
		}
	} else {
		// If no properties, required should be empty
		if _, hasRequired := schema["required"]; hasRequired {
			schema["required"] = []interface{}{}
		}
	}

	// 7. Handle type field (Gemini requires single string and lowercase)
	if typeVal, ok := schema["type"]; ok {
		switch t := typeVal.(type) {
		case string:
			schema["type"] = strings.ToLower(t)
		case []interface{}:
			// Union type: ["string", "null"] -> "string"
			selectedType := "string"
			for _, item := range t {
				if s, ok := item.(string); ok && s != "null" {
					selectedType = strings.ToLower(s)
					break
				}
			}
			schema["type"] = selectedType
		}
	}

	// 8. [FIX #374] Ensure enum values are all strings
	// Gemini v1internal strictly requires all enum elements to be TYPE_STRING
	// MCP tool definitions may contain numeric or boolean enums that need conversion
	if enumVal, ok := schema["enum"].([]interface{}); ok {
		for i, item := range enumVal {
			switch v := item.(type) {
			case string:
				// Already string, keep as is
			case float64:
				enumVal[i] = fmt.Sprintf("%v", v)
			case int:
				enumVal[i] = fmt.Sprintf("%d", v)
			case bool:
				enumVal[i] = fmt.Sprintf("%v", v)
			case nil:
				enumVal[i] = "null"
			default:
				// Complex types convert to JSON string
				if b, err := json.Marshal(v); err == nil {
					enumVal[i] = string(b)
				}
			}
		}
	}
}

// extractTypeFromUnion extracts the first non-null type from anyOf/oneOf array
// (like Antigravity-Manager's extract_type_from_union)
//
// Example: anyOf: [{"type": "string"}, {"type": "null"}] -> "string"
// Example: anyOf: [{"type": "integer"}, {"type": "null"}] -> "integer"
// Example: anyOf: [{"type": "null"}] -> "" (only null)
func extractTypeFromUnion(unionArray []interface{}) string {
	for _, item := range unionArray {
		if obj, ok := item.(map[string]interface{}); ok {
			if typeStr, ok := obj["type"].(string); ok {
				// Skip null type, take first non-null type
				if typeStr != "null" {
					return strings.ToLower(typeStr)
				}
			}
		}
	}
	// If all are null or can't extract, return empty
	return ""
}

// CleanToolInputSchema cleans a tool's input schema for Gemini compatibility
// This is a convenience wrapper around CleanJSONSchema
func CleanToolInputSchema(inputSchema interface{}) map[string]interface{} {
	if inputSchema == nil {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	schema, ok := inputSchema.(map[string]interface{})
	if !ok {
		return map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	// Deep copy to avoid modifying original
	copied := deepCopyValue(schema).(map[string]interface{})

	CleanJSONSchema(copied)

	return copied
}
