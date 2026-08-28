package location

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xen0bit/pwrq/pkg/core/sessionstate"
)

// createTestSessionState creates a new session state for testing
func createTestSessionState() *sessionstate.SessionState {
	return sessionstate.NewSessionState()
}

func TestGetLocation(t *testing.T) {
	// Get current directory
	expected, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	ss := createTestSessionState()
	result, err := getLocation(GetLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("getLocation failed: %v", err)
	}

	if result["Path"] != expected {
		t.Errorf("Expected path %q, got %q", expected, result["Path"])
	}

	if result["Provider"] != "FileSystem" {
		t.Errorf("Expected Provider to be FileSystem, got %q", result["Provider"])
	}
}

func TestGetLocationWithDrive(t *testing.T) {
	expected, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}

	ss := createTestSessionState()
	result, err := getLocation(GetLocationOptions{Drive: "FileSystem"}, ss)
	if err != nil {
		t.Fatalf("getLocation failed: %v", err)
	}

	if result["Path"] != expected {
		t.Errorf("Expected path %q, got %q", expected, result["Path"])
	}

	if result["Drive"] != "FileSystem" {
		t.Errorf("Expected Drive to be FileSystem, got %q", result["Drive"])
	}
}

func TestGetLocationWithStackName(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	ss := createTestSessionState()

	// Push a location onto a named stack
	testDir := t.TempDir()
	ss.PushLocationStack("testStack", testDir)

	// Get location from the named stack
	result, err := getLocation(GetLocationOptions{StackName: "testStack"}, ss)
	if err != nil {
		t.Fatalf("getLocation with StackName failed: %v", err)
	}

	expected, _ := filepath.Abs(testDir)
	if result["Path"] != expected {
		t.Errorf("Expected path %q, got %q", expected, result["Path"])
	}

	if result["StackName"] != "testStack" {
		t.Errorf("Expected StackName to be testStack, got %q", result["StackName"])
	}
}

func TestGetLocationWithEmptyStack(t *testing.T) {
	ss := createTestSessionState()

	_, err := getLocation(GetLocationOptions{StackName: "nonExistentStack"}, ss)
	if err == nil {
		t.Error("Expected error for empty/non-existent stack, got nil")
	}
}

func TestSetLocation(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	ss := createTestSessionState()

	// Change to the temporary directory
	resultPath, err := setLocation(tmpDir, SetLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("setLocation failed: %v", err)
	}

	// Verify the path
	expected, _ := filepath.Abs(tmpDir)
	if resultPath != expected {
		t.Errorf("Expected path %q, got %q", expected, resultPath)
	}

	// Verify we're actually in the new directory
	cwd, _ := os.Getwd()
	if cwd != expected {
		t.Errorf("Expected to be in %q, but got %q", expected, cwd)
	}

	// Verify PWD variable was updated
	pwd, err := ss.GetVariable("PWD")
	if err != nil {
		t.Errorf("PWD variable not set: %v", err)
	}
	if pwd != expected {
		t.Errorf("Expected PWD to be %q, got %q", expected, pwd)
	}
}

func TestSetLocationWithStackName(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	tmpDir := t.TempDir()
	ss := createTestSessionState()

	// Set location with StackName - should push current location first
	_, err = setLocation(tmpDir, SetLocationOptions{StackName: "testStack"}, ss)
	if err != nil {
		t.Fatalf("setLocation with StackName failed: %v", err)
	}

	// Verify the original location was pushed onto the stack
	stack := ss.GetLocationStack("testStack")
	if len(stack) != 1 {
		t.Errorf("Expected stack size 1, got %d", len(stack))
	}
	if stack[0] != original {
		t.Errorf("Expected stack[0] to be %q, got %q", original, stack[0])
	}
}

func TestSetLocationInvalidPath(t *testing.T) {
	ss := createTestSessionState()
	_, err := setLocation("/nonexistent/path/that/does/not/exist", SetLocationOptions{}, ss)
	if err == nil {
		t.Error("Expected error for invalid path, got nil")
	}
}

func TestSetLocationEmptyPath(t *testing.T) {
	ss := createTestSessionState()
	_, err := setLocation("", SetLocationOptions{}, ss)
	if err == nil {
		t.Error("Expected error for empty path, got nil")
	}
}

func TestPushLocation(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	ss := createTestSessionState()

	// Create a temporary directory
	tmpDir := t.TempDir()

	// Push current location and change to tmpDir
	popped, target, err := pushLocation(tmpDir, PushLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("pushLocation failed: %v", err)
	}

	// Verify the pushed location matches original
	if popped != original {
		t.Errorf("Expected pushed location %q, got %q", original, popped)
	}

	// Verify the target location
	expected, _ := filepath.Abs(tmpDir)
	if target != expected {
		t.Errorf("Expected target path %q, got %q", expected, target)
	}

	// Verify we're actually in the new directory
	cwd, _ := os.Getwd()
	if cwd != expected {
		t.Errorf("Expected to be in %q, but got %q", expected, cwd)
	}

	// Verify the stack has the original location
	stack := GetLocationStack(ss, "")
	if len(stack) != 1 {
		t.Errorf("Expected stack size 1, got %d", len(stack))
	}
	if stack[0] != original {
		t.Errorf("Expected stack[0] to be %q, got %q", original, stack[0])
	}
}

func TestPushLocationNoPath(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	ss := createTestSessionState()

	// Push without changing directory
	popped, target, err := pushLocation("", PushLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("pushLocation failed: %v", err)
	}

	// Both should be the original directory
	if popped != original {
		t.Errorf("Expected pushed location %q, got %q", original, popped)
	}
	if target != original {
		t.Errorf("Expected target location %q, got %q", original, target)
	}

	// Verify stack
	stack := GetLocationStack(ss, "")
	if len(stack) != 1 {
		t.Errorf("Expected stack size 1, got %d", len(stack))
	}
}

func TestPushLocationNamedStack(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	ss := createTestSessionState()
	testStack := "testStack"
	tmpDir := t.TempDir()

	// Push to named stack
	_, _, err = pushLocation(tmpDir, PushLocationOptions{StackName: testStack}, ss)
	if err != nil {
		t.Fatalf("pushLocation failed: %v", err)
	}

	// Verify default stack is unchanged (empty)
	defaultStack := GetLocationStack(ss, "")
	if len(defaultStack) != 0 {
		t.Errorf("Expected default stack to be empty, got %v", defaultStack)
	}

	// Verify named stack has the location
	namedStack := GetLocationStack(ss, testStack)
	if len(namedStack) != 1 {
		t.Errorf("Expected named stack size 1, got %d", len(namedStack))
	}
	if namedStack[0] != original {
		t.Errorf("Expected named stack[0] to be %q, got %q", original, namedStack[0])
	}
}

func TestPopLocation(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	ss := createTestSessionState()

	// Set up the stack
	tmpDir := t.TempDir()
	ss.PushLocationStack("default", original)

	// Change to a different directory first
	_ = os.Chdir(tmpDir)

	// Pop location
	resultPath, err := popLocation(PopLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("popLocation failed: %v", err)
	}

	// Verify we're back to the original directory
	if resultPath != original {
		t.Errorf("Expected path %q, got %q", original, resultPath)
	}

	// Verify we're actually in the original directory
	cwd, _ := os.Getwd()
	if cwd != original {
		t.Errorf("Expected to be in %q, but got %q", original, cwd)
	}

	// Verify stack is empty
	stack := GetLocationStack(ss, "")
	if len(stack) != 0 {
		t.Errorf("Expected stack to be empty, got %v", stack)
	}
}

func TestPopLocationEmptyStack(t *testing.T) {
	ss := createTestSessionState()

	_, err := popLocation(PopLocationOptions{}, ss)
	if err == nil {
		t.Error("Expected error for empty stack, got nil")
	}
}

func TestPopLocationNonExistentStack(t *testing.T) {
	ss := createTestSessionState()

	_, err := popLocation(PopLocationOptions{StackName: "nonExistentStack"}, ss)
	if err == nil {
		t.Error("Expected error for non-existent stack, got nil")
	}
}

func TestPopLocationNamedStack(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	ss := createTestSessionState()
	testStack := "testStack"

	// Set up named stack
	ss.PushLocationStack(testStack, original)
	ss.PushLocationStack("default", "/some/other/path")

	tmpDir := t.TempDir()
	_ = os.Chdir(tmpDir)

	// Pop from named stack
	resultPath, err := popLocation(PopLocationOptions{StackName: testStack}, ss)
	if err != nil {
		t.Fatalf("popLocation failed: %v", err)
	}

	// Verify we're back to the original directory from named stack
	if resultPath != original {
		t.Errorf("Expected path %q, got %q", original, resultPath)
	}

	// Verify default stack is unchanged
	defaultStack := GetLocationStack(ss, "")
	if len(defaultStack) != 1 {
		t.Errorf("Expected default stack size 1, got %d", len(defaultStack))
	}

	// Verify named stack is empty
	namedStack := GetLocationStack(ss, testStack)
	if len(namedStack) != 0 {
		t.Errorf("Expected named stack to be empty, got %v", namedStack)
	}
}

func TestPushPopSequence(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	ss := createTestSessionState()

	// Create test directories
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	// Push to tmpDir1
	_, _, err = pushLocation(tmpDir1, PushLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("First pushLocation failed: %v", err)
	}

	// Push to tmpDir2
	_, _, err = pushLocation(tmpDir2, PushLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("Second pushLocation failed: %v", err)
	}

	// Verify stack size
	stack := GetLocationStack(ss, "")
	if len(stack) != 2 {
		t.Errorf("Expected stack size 2, got %d", len(stack))
	}

	// Pop back to tmpDir1
	_, err = popLocation(PopLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("First popLocation failed: %v", err)
	}

	cwd, _ := os.Getwd()
	expected1, _ := filepath.Abs(tmpDir1)
	if cwd != expected1 {
		t.Errorf("Expected to be in %q after first pop, got %q", expected1, cwd)
	}

	// Pop back to original
	_, err = popLocation(PopLocationOptions{}, ss)
	if err != nil {
		t.Fatalf("Second popLocation failed: %v", err)
	}

	cwd, _ = os.Getwd()
	if cwd != original {
		t.Errorf("Expected to be in %q after second pop, got %q", original, cwd)
	}
}

func TestPopLocationNonExistentDirectory(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	ss := createTestSessionState()

	// Create a temporary directory and push it
	tmpDir := t.TempDir()
	ss.PushLocationStack("default", tmpDir)

	// Remove the directory
	_ = os.RemoveAll(tmpDir)

	// Try to pop to the non-existent directory
	_, err = popLocation(PopLocationOptions{}, ss)
	if err == nil {
		t.Error("Expected error for non-existent directory, got nil")
	}
}

func TestSessionIsolation(t *testing.T) {
	// Save original directory
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer func() { _ = os.Chdir(original) }()

	// Create two separate session states
	ss1 := createTestSessionState()
	ss2 := createTestSessionState()

	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	// Push to ss1's stack (this changes cwd to tmpDir1)
	_, _, err = pushLocation(tmpDir1, PushLocationOptions{}, ss1)
	if err != nil {
		t.Fatalf("pushLocation to ss1 failed: %v", err)
	}

	// Save cwd after first push
	cwdAfterFirst, _ := os.Getwd()

	// Push to ss2's stack (this changes cwd to tmpDir2)
	_, _, err = pushLocation(tmpDir2, PushLocationOptions{}, ss2)
	if err != nil {
		t.Fatalf("pushLocation to ss2 failed: %v", err)
	}

	// Verify ss1's stack - should have original directory
	stack1 := GetLocationStack(ss1, "")
	if len(stack1) != 1 {
		t.Errorf("Expected ss1 stack size 1, got %d", len(stack1))
	}
	if stack1[0] != original {
		t.Errorf("Expected ss1 stack[0] to be %q, got %q", original, stack1[0])
	}

	// Verify ss2's stack - should have cwd after first push (tmpDir1)
	stack2 := GetLocationStack(ss2, "")
	if len(stack2) != 1 {
		t.Errorf("Expected ss2 stack size 1, got %d", len(stack2))
	}
	if stack2[0] != cwdAfterFirst {
		t.Errorf("Expected ss2 stack[0] to be %q, got %q", cwdAfterFirst, stack2[0])
	}

	// Verify stacks are independent - they should have different values
	if stack1[0] == stack2[0] {
		t.Error("Stacks should have different values (sessions are independent)")
	}
}
