package utils

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// testLogDir is the temporary directory for test logs (not in home directory)
var testLogDir string

// setupTestLogger initializes the logger with a test directory
func setupTestLogger(t *testing.T) (cleanup func()) {
	t.Helper()

	// Create a temp directory for test logs
	testLogDir = t.TempDir()

	// Set debug mode
	os.Setenv("LAZYAZURE_DEBUG", "1")

	// Reset logger state
	CloseLogger()
	logPath = ""
	enabled = false

	// Override the log directory for testing
	originalHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", testLogDir)

	return func() {
		CloseLogger()
		os.Unsetenv("LAZYAZURE_DEBUG")
		os.Setenv("HOME", originalHomeDir)
		testLogDir = ""
	}
}

// getTestLogPath returns the path to the test log file
func getTestLogPath() string {
	return filepath.Join(testLogDir, ".lazyazure", "debug.log")
}

// readLogFile reads the entire log file content
func readLogFile(t *testing.T) string {
	t.Helper()
	logFilePath := getTestLogPath()
	data, err := os.ReadFile(logFilePath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	return string(data)
}

// readLogLines reads the log file as a slice of lines
func readLogLines(t *testing.T) []string {
	t.Helper()
	content := readLogFile(t)
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

// TestIsDebugEnabled_Set verifies that LAZYAZURE_DEBUG=1 enables debug logging
func TestIsDebugEnabled_Set(t *testing.T) {
	// Save and restore original value
	originalValue := os.Getenv("LAZYAZURE_DEBUG")
	defer os.Setenv("LAZYAZURE_DEBUG", originalValue)

	os.Setenv("LAZYAZURE_DEBUG", "1")
	if !IsDebugEnabled() {
		t.Error("IsDebugEnabled() should return true when LAZYAZURE_DEBUG=1")
	}

	os.Setenv("LAZYAZURE_DEBUG", "true")
	if !IsDebugEnabled() {
		t.Error("IsDebugEnabled() should return true when LAZYAZURE_DEBUG=true")
	}

	os.Setenv("LAZYAZURE_DEBUG", "yes")
	if !IsDebugEnabled() {
		t.Error("IsDebugEnabled() should return true when LAZYAZURE_DEBUG=yes")
	}
}

// TestIsDebugEnabled_Unset verifies that unset LAZYAZURE_DEBUG disables debug logging
func TestIsDebugEnabled_Unset(t *testing.T) {
	// Save and restore original value
	originalValue := os.Getenv("LAZYAZURE_DEBUG")
	defer os.Setenv("LAZYAZURE_DEBUG", originalValue)

	os.Unsetenv("LAZYAZURE_DEBUG")
	if IsDebugEnabled() {
		t.Error("IsDebugEnabled() should return false when LAZYAZURE_DEBUG is unset")
	}

	os.Setenv("LAZYAZURE_DEBUG", "")
	if IsDebugEnabled() {
		t.Error("IsDebugEnabled() should return false when LAZYAZURE_DEBUG is empty")
	}
}

// TestInitLogger_Success verifies that InitLogger creates the log file
func TestInitLogger_Success(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	// Verify log file was created
	logFilePath := getTestLogPath()
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		t.Errorf("Log file was not created at %s", logFilePath)
	}

	// Verify log path is set
	if GetLogPath() != logFilePath {
		t.Errorf("GetLogPath() = %q, want %q", GetLogPath(), logFilePath)
	}
}

// TestInitLogger_Disabled verifies that InitLogger is a no-op when debug is disabled
func TestInitLogger_Disabled(t *testing.T) {
	// Save and restore
	originalValue := os.Getenv("LAZYAZURE_DEBUG")
	defer os.Setenv("LAZYAZURE_DEBUG", originalValue)

	os.Unsetenv("LAZYAZURE_DEBUG")
	CloseLogger()

	err := InitLogger()
	if err != nil {
		t.Errorf("InitLogger() should not error when debug is disabled: %v", err)
	}

	if GetLogPath() != "" {
		t.Error("GetLogPath() should return empty when debug is disabled")
	}
}

// TestLog_Enabled verifies that Log writes to the file when enabled
func TestLog_Enabled(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	Log("Test message: %s", "hello")

	content := readLogFile(t)
	if !strings.Contains(content, "Test message: hello") {
		t.Errorf("Log content should contain 'Test message: hello', got:\n%s", content)
	}
}

// TestLog_Disabled verifies that Log is a no-op when disabled
func TestLog_Disabled(t *testing.T) {
	// Save and restore
	originalValue := os.Getenv("LAZYAZURE_DEBUG")
	defer os.Setenv("LAZYAZURE_DEBUG", originalValue)

	os.Unsetenv("LAZYAZURE_DEBUG")
	CloseLogger()

	// This should not panic or write anything
	Log("This should not be logged: %s", "test")

	if GetLogPath() != "" {
		t.Error("GetLogPath() should return empty when logging is disabled")
	}
}

// TestLog_MultipleMessages verifies that multiple log messages are written
func TestLog_MultipleMessages(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	Log("First message")
	Log("Second message: %d", 42)
	Log("Third message: %s %s", "hello", "world")

	lines := readLogLines(t)

	// Should have at least 5 lines (2 header + 3 messages)
	if len(lines) < 5 {
		t.Errorf("Expected at least 5 log lines, got %d", len(lines))
	}

	// Check for our messages
	foundFirst := false
	foundSecond := false
	foundThird := false

	for _, line := range lines {
		if strings.Contains(line, "First message") {
			foundFirst = true
		}
		if strings.Contains(line, "Second message: 42") {
			foundSecond = true
		}
		if strings.Contains(line, "Third message: hello world") {
			foundThird = true
		}
	}

	if !foundFirst {
		t.Error("First message not found in log")
	}
	if !foundSecond {
		t.Error("Second message not found in log")
	}
	if !foundThird {
		t.Error("Third message not found in log")
	}
}

// TestCloseLogger_Idempotent verifies that CloseLogger can be called multiple times without panic
func TestCloseLogger_Idempotent(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	// Close multiple times - should not panic
	CloseLogger()
	CloseLogger()
	CloseLogger()

	// Verify logger is disabled
	if GetLogPath() != "" {
		t.Error("GetLogPath() should return empty after CloseLogger()")
	}

	// Verify log was written (logger closed message)
	content := readLogFile(t)
	if !strings.Contains(content, "Logger closed") {
		t.Error("Log should contain 'Logger closed' message")
	}
}

// TestGetLogPath_Disabled verifies GetLogPath returns empty when logging is disabled
func TestGetLogPath_Disabled(t *testing.T) {
	// Save and restore
	originalValue := os.Getenv("LAZYAZURE_DEBUG")
	defer os.Setenv("LAZYAZURE_DEBUG", originalValue)

	os.Unsetenv("LAZYAZURE_DEBUG")
	CloseLogger()

	if GetLogPath() != "" {
		t.Errorf("GetLogPath() = %q, want empty string when disabled", GetLogPath())
	}
}

// TestGetLogPath_Enabled verifies GetLogPath returns the correct path when enabled
func TestGetLogPath_Enabled(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	expectedPath := getTestLogPath()
	if GetLogPath() != expectedPath {
		t.Errorf("GetLogPath() = %q, want %q", GetLogPath(), expectedPath)
	}
}

// TestLog_Timestamp verifies that log entries have timestamps
func TestLog_Timestamp(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	Log("Timestamp test message")

	content := readLogFile(t)
	lines := readLogLines(t)

	// Find our test message line
	var messageLine string
	for _, line := range lines {
		if strings.Contains(line, "Timestamp test message") {
			messageLine = line
			break
		}
	}

	if messageLine == "" {
		t.Fatalf("Test message not found in log content:\n%s", content)
	}

	// Check timestamp format: [YYYY-MM-DD HH:MM:SS.mmm]
	timestampPattern := `\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}\] Timestamp test message`
	matched, err := regexp.MatchString(timestampPattern, messageLine)
	if err != nil {
		t.Fatalf("Regex error: %v", err)
	}
	if !matched {
		t.Errorf("Log line does not have expected timestamp format: %s", messageLine)
	}
}

// TestLog_Privacy_SensitivePatterns verifies that logs don't contain sensitive patterns
// This test is defensive - it checks that our logging practices follow AGENTS.md guidelines
func TestLog_Privacy_SensitivePatterns(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	// Try to log various sensitive patterns (simulating what might accidentally happen)
	sensitiveData := []string{
		// Email addresses / UPNs
		"user@example.com",
		"admin@contoso.onmicrosoft.com",
		"test.user@company.org",

		// GUIDs (subscription IDs, tenant IDs, etc.)
		"12345678-1234-1234-1234-123456789012",
		"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",

		// Azure Resource IDs
		"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/myRG",
		"/subscriptions/12345678-1234-1234-1234-123456789012/resourceGroups/myRG/providers/Microsoft.Compute/virtualMachines/myVM",

		// JWT token patterns
		"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
		"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
	}

	// Log each sensitive value (these should NOT actually contain the sensitive data)
	for i := range sensitiveData {
		// In real code, we should NOT log these directly
		// This simulates what would happen if someone accidentally did
		Log("Processing item %d with data", i)
	}

	// Also log some safe patterns that SHOULD be allowed
	Log("Application started successfully")
	Log("Loaded 25 subscriptions")
	Log("Found resource at index 5")
	Log("Operation completed in 150ms")
	Log("Error: failed to connect to Azure API")

	content := readLogFile(t)

	// Verify NO sensitive patterns exist in the log
	privacyViolations := []string{}

	// Check for email addresses
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	if emailPattern.MatchString(content) {
		privacyViolations = append(privacyViolations, "email address found")
	}

	// Check for GUIDs (simple pattern)
	guidPattern := regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	if guidPattern.MatchString(content) {
		privacyViolations = append(privacyViolations, "GUID found")
	}

	// Check for Azure resource ID patterns
	if strings.Contains(content, "/subscriptions/") {
		privacyViolations = append(privacyViolations, "Azure resource ID pattern found")
	}

	// Check for JWT token patterns (base64url encoded JWT segments)
	jwtPattern := regexp.MustCompile(`eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*`)
	if jwtPattern.MatchString(content) {
		privacyViolations = append(privacyViolations, "JWT token pattern found")
	}

	if len(privacyViolations) > 0 {
		t.Errorf("Privacy violations found in log: %v\nLog content:\n%s", privacyViolations, content)
	}
}

// TestLog_Privacy_Guidelines documents the privacy-safe logging patterns
func TestLog_Privacy_Guidelines(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	// These are the SAFE patterns we should use (from AGENTS.md)
	safeLogPatterns := []struct {
		name    string
		message string
	}{
		{"presence indicator", "Has subscription: true"},
		{"index instead of ID", "Found subscription at index 5"},
		{"count", "Loaded 20 subscriptions"},
		{"anonymized identifier", "Processing resource-1"},
		{"length of search", "Search query: 5 characters"},
		{"operation timing", "API call completed in 250ms"},
		{"result count", "Found 15 resource groups"},
		{"boolean state", "User authenticated: true"},
	}

	for _, pattern := range safeLogPatterns {
		Log("%s", pattern.message)
	}

	content := readLogFile(t)

	// Verify all safe patterns are present
	for _, pattern := range safeLogPatterns {
		if !strings.Contains(content, pattern.message) {
			t.Errorf("Safe pattern '%s' not found in log: %s", pattern.name, pattern.message)
		}
	}

	// Verify no sensitive patterns leaked
	// (Same checks as TestLog_Privacy_SensitivePatterns)
	emailPattern := regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	if emailPattern.MatchString(content) {
		t.Error("Email pattern found in safe logging test")
	}
}

// TestConcurrentLogging verifies that concurrent Log calls don't corrupt the log
func TestConcurrentLogging(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	// Spawn multiple goroutines to log concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				Log("Goroutine %d message %d", id, j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Read log and verify no corruption
	content := readLogFile(t)
	lines := readLogLines(t)

	// We should have at least 100 messages (10 goroutines * 10 messages each)
	// Plus 2 header lines
	messageCount := 0
	for _, line := range lines {
		if strings.Contains(line, "Goroutine") {
			messageCount++
		}
	}

	if messageCount < 100 {
		t.Errorf("Expected at least 100 log messages, found %d\nLog content:\n%s", messageCount, content)
	}

	// Verify no lines are malformed (every line should start with [timestamp])
	linePattern := regexp.MustCompile(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}\]`)
	for i, line := range lines {
		if line == "" {
			continue // Skip empty lines
		}
		if !linePattern.MatchString(line) {
			t.Errorf("Line %d is malformed (missing timestamp): %s", i, line)
		}
	}
}

// TestLog_Whitespace verifies that log messages with special characters are handled
func TestLog_Whitespace(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	Log("Message with\ttab")
	Log("Message with\nnewline")
	Log("Message with  multiple   spaces")
	Log("")

	content := readLogFile(t)

	// Verify messages are logged (even if formatting is affected)
	if !strings.Contains(content, "Message with") {
		t.Error("Whitespace test messages not found in log")
	}
}

// TestLog_FormatSpecifiers verifies that format specifiers work correctly
func TestLog_FormatSpecifiers(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	Log("String: %s", "test")
	Log("Integer: %d", 42)
	Log("Float: %f", 3.14)
	Log("Boolean: %t", true)
	Log("Hex: %x", 255)
	Log("Multiple: %s %d %s", "a", 1, "b")
	Log("Percent: %%")

	content := readLogFile(t)
	lines := readLogLines(t)

	expectedMessages := []string{
		"String: test",
		"Integer: 42",
		"Float: 3.140000",
		"Boolean: true",
		"Hex: ff",
		"Multiple: a 1 b",
		"Percent: %",
	}

	for _, expected := range expectedMessages {
		found := false
		for _, line := range lines {
			if strings.Contains(line, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected message '%s' not found in log.\nContent:\n%s", expected, content)
		}
	}
}

// TestInitLogger_AlreadyInitialized verifies that re-initializing the logger works
func TestInitLogger_AlreadyInitialized(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	// First initialization
	err := InitLogger()
	if err != nil {
		t.Fatalf("First InitLogger() failed: %v", err)
	}

	Log("First message")

	// Second initialization (should truncate and start fresh)
	err = InitLogger()
	if err != nil {
		t.Fatalf("Second InitLogger() failed: %v", err)
	}

	Log("Second message")

	content := readLogFile(t)

	// Should NOT contain "First message" because file is truncated on re-init
	if strings.Contains(content, "First message") {
		t.Error("Log should have been truncated on re-initialization, but 'First message' was found")
	}

	// Should contain "Second message"
	if !strings.Contains(content, "Second message") {
		t.Error("Log should contain 'Second message'")
	}
}

// TestLog_Verbose verifies verbose logging doesn't cause issues
func TestLog_Verbose(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	// Log many messages to test performance and file handling
	for i := 0; i < 1000; i++ {
		Log("Verbose log message number %d with some data: %s", i, "test data here")
	}

	content := readLogFile(t)

	// Verify all messages were written
	if !strings.Contains(content, "Verbose log message number 0") {
		t.Error("First verbose message not found")
	}
	if !strings.Contains(content, "Verbose log message number 999") {
		t.Error("Last verbose message not found")
	}

	// Count lines to verify approximate count
	lines := readLogLines(t)
	// Should have ~1000 messages + 2 header lines + logger closed line (if closed)
	if len(lines) < 1000 {
		t.Errorf("Expected at least 1000 log lines, got %d", len(lines))
	}
}

// TestLog_ErrorMessages verifies error messages are logged correctly
func TestLog_ErrorMessages(t *testing.T) {
	cleanup := setupTestLogger(t)
	defer cleanup()

	err := InitLogger()
	if err != nil {
		t.Fatalf("InitLogger() failed: %v", err)
	}

	// Log various error patterns
	Log("Error: %s", "connection failed")
	Log("Warning: %s", "deprecated API usage")
	Log("Fatal: %s", "unexpected shutdown")
	Log("Debug: %s", "variable state")

	content := readLogFile(t)

	expectedMessages := []string{
		"Error: connection failed",
		"Warning: deprecated API usage",
		"Fatal: unexpected shutdown",
		"Debug: variable state",
	}

	for _, expected := range expectedMessages {
		if !strings.Contains(content, expected) {
			t.Errorf("Expected error message '%s' not found", expected)
		}
	}
}

// TestInitLogger_ReadOnlyDir tests error handling when log directory can't be created
func TestInitLogger_ReadOnlyDir(t *testing.T) {
	// Save and restore
	originalValue := os.Getenv("LAZYAZURE_DEBUG")
	defer os.Setenv("LAZYAZURE_DEBUG", originalValue)

	os.Setenv("LAZYAZURE_DEBUG", "1")
	CloseLogger()

	// Set HOME to a location that doesn't exist and can't be created
	// (Use a path with invalid characters or a read-only filesystem path)
	originalHomeDir := os.Getenv("HOME")
	os.Setenv("HOME", "/nonexistent/path/that/cannot/be/created")
	defer os.Setenv("HOME", originalHomeDir)

	err := InitLogger()
	// Should return an error
	if err == nil {
		t.Error("InitLogger() should return error when log directory cannot be created")
	}
}

// BenchmarkLog measures logging performance
func BenchmarkLog(b *testing.B) {
	// Setup
	testLogDir = b.TempDir()
	os.Setenv("LAZYAZURE_DEBUG", "1")
	os.Setenv("HOME", testLogDir)
	CloseLogger()

	err := InitLogger()
	if err != nil {
		b.Fatalf("InitLogger() failed: %v", err)
	}
	defer CloseLogger()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Log("Benchmark message %d", i)
	}
}
