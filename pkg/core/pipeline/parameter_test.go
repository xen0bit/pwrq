package pipeline

import (
	"reflect"
	"testing"
)

// TestParameter tests parameter binding.
func TestParameter(t *testing.T) {
	t.Run("binds matching types", func(t *testing.T) {
		var target string
		targetValue := reflect.ValueOf(&target).Elem()

		err := Parameter("test", "hello", targetValue, ParameterAttribute{})
		if err != nil {
			t.Fatalf("Parameter failed: %v", err)
		}
		if target != "hello" {
			t.Errorf("expected 'hello', got %q", target)
		}
	})

	t.Run("converts string to int", func(t *testing.T) {
		var target int
		targetValue := reflect.ValueOf(&target).Elem()

		err := Parameter("test", "42", targetValue, ParameterAttribute{})
		if err != nil {
			t.Fatalf("Parameter failed: %v", err)
		}
		if target != 42 {
			t.Errorf("expected 42, got %d", target)
		}
	})

	t.Run("converts string to bool", func(t *testing.T) {
		var target bool
		targetValue := reflect.ValueOf(&target).Elem()

		err := Parameter("test", "true", targetValue, ParameterAttribute{})
		if err != nil {
			t.Fatalf("Parameter failed: %v", err)
		}
		if !target {
			t.Errorf("expected true, got %v", target)
		}
	})

	t.Run("rejects mandatory nil value", func(t *testing.T) {
		var target string
		targetValue := reflect.ValueOf(&target).Elem()

		err := Parameter("test", nil, targetValue, ParameterAttribute{Mandatory: true})
		if err == nil {
			t.Fatal("expected error for mandatory parameter, got nil")
		}
		if err.Error() != `parameter "test" is mandatory` {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("accepts optional nil value", func(t *testing.T) {
		var target string
		targetValue := reflect.ValueOf(&target).Elem()

		err := Parameter("test", nil, targetValue, ParameterAttribute{Mandatory: false})
		if err != nil {
			t.Fatalf("Parameter failed for optional nil: %v", err)
		}
	})
}

// TestBindParameters tests struct parameter binding.
func TestBindParameters(t *testing.T) {
	t.Run("binds named parameters", func(t *testing.T) {
		type TestParams struct {
			Name  string `param:"name"`
			Value int    `param:"value"`
		}

		params := &TestParams{}
		err := BindParameters(map[string]any{
			"name":  "test",
			"value": 42,
		}, params)

		if err != nil {
			t.Fatalf("BindParameters failed: %v", err)
		}
		if params.Name != "test" {
			t.Errorf("expected Name='test', got %q", params.Name)
		}
		if params.Value != 42 {
			t.Errorf("expected Value=42, got %d", params.Value)
		}
	})

	t.Run("validates mandatory parameters", func(t *testing.T) {
		type TestParams struct {
			Name string `param:"name,mandatory"`
		}

		params := &TestParams{}
		err := BindParameters(map[string]any{}, params)

		if err == nil {
			t.Fatal("expected error for missing mandatory parameter")
		}
		if err.Error() != "missing mandatory parameter: name" {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("accepts present mandatory parameters", func(t *testing.T) {
		type TestParams struct {
			Name string `param:"name,mandatory"`
		}

		params := &TestParams{}
		err := BindParameters(map[string]any{
			"name": "test",
		}, params)

		if err != nil {
			t.Fatalf("BindParameters failed: %v", err)
		}
		if params.Name != "test" {
			t.Errorf("expected Name='test', got %q", params.Name)
		}
	})

	t.Run("handles nil params map", func(t *testing.T) {
		type TestParams struct {
			Name string `param:"name"`
		}

		params := &TestParams{}
		err := BindParameters(nil, params)

		if err != nil {
			t.Fatalf("BindParameters failed with nil params: %v", err)
		}
	})

	t.Run("rejects non-struct target", func(t *testing.T) {
		var target string
		err := BindParameters(map[string]any{}, &target)

		if err == nil {
			t.Fatal("expected error for non-struct target")
		}
	})
}

// TestBindParametersWithPosition tests positional parameter binding.
func TestBindParametersWithPosition(t *testing.T) {
	t.Run("parses positional tags", func(t *testing.T) {
		type TestParams struct {
			Path    string `param:"path,pos=0"`
			Recurse bool   `param:"recurse,pos=1"`
		}

		params := &TestParams{}
		// Note: Current implementation binds named params only
		// Positional binding would require an ordered argument list
		err := BindParameters(map[string]any{
			"path": "/test",
		}, params)

		if err != nil {
			t.Fatalf("BindParameters failed: %v", err)
		}
		if params.Path != "/test" {
			t.Errorf("expected Path='/test', got %q", params.Path)
		}
	})
}
