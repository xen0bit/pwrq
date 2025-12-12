package common

// IsUDFResult checks if a value is a UDF result object (has _val and _meta keys)
func IsUDFResult(v any) bool {
	obj, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, hasVal := obj["_val"]
	_, hasMeta := obj["_meta"]
	return hasVal && hasMeta
}

// ExtractUDFValue extracts the _val from a UDF result object, or returns the value as-is
// This allows UDFs to automatically unwrap _val when chaining UDFs together.
// This is the standard behavior for all UDFs - if a UDF receives a UDF result object
// and doesn't need to access _meta, it should automatically extract _val.
func ExtractUDFValue(v any) any {
	if IsUDFResult(v) {
		obj := v.(map[string]any)
		return obj["_val"]
	}
	return v
}

