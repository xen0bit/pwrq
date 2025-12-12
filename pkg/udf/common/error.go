package common

// MakeUDFErrorResult creates a UDF result object with an error
// Returns {_val: null, _meta: {...}, _err: errorMessage}
func MakeUDFErrorResult(err error, meta map[string]any) map[string]any {
	if meta == nil {
		meta = make(map[string]any)
	}
	
	result := map[string]any{
		"_val":  nil,
		"_meta": meta,
		"_err":  err.Error(),
	}
	
	return result
}

// MakeUDFSuccessResult creates a UDF result object with a value
// Returns {_val: value, _meta: {...}}
func MakeUDFSuccessResult(value any, meta map[string]any) map[string]any {
	if meta == nil {
		meta = make(map[string]any)
	}
	
	result := map[string]any{
		"_val":  value,
		"_meta": meta,
	}
	
	return result
}

