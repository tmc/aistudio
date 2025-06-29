# Test Logging in aistudio

This document describes the test logging infrastructure that ensures all log output is properly connected to `testing.T.Logf` when running tests.

## Overview

The aistudio codebase uses the standard `log` package for logging throughout the application. When running tests, it's important that these log messages are captured and associated with the correct test case. This is achieved through our test logging helpers.

## How to Use

### Basic Usage

At the beginning of any test function that might trigger logging (directly or indirectly), add:

```go
func TestSomething(t *testing.T) {
    cleanup := SetupTestLogging(t)
    defer cleanup()
    
    // Your test code here
}
```

### For External Test Packages

If your test is in a `_test` package (e.g., `aistudio_test`), the helper functions are duplicated in `test_helpers_external_test.go`:

```go
func TestSomething(t *testing.T) {
    cleanup := SetupTestLogging(t)
    defer cleanup()
    
    // Your test code here
}
```

### Capturing Log Output

If you need to capture and assert on log output:

```go
func TestLogging(t *testing.T) {
    output := captureLogOutput(func() {
        log.Println("test message")
    })
    
    if !strings.Contains(output, "test message") {
        t.Errorf("Expected log output to contain 'test message'")
    }
}
```

### Advanced: Capture and Display

To both capture log output for assertions AND display it in test logs:

```go
func TestWithCapture(t *testing.T) {
    buffer, cleanup := SetupTestLoggingWithCapture(t)
    defer cleanup()
    
    // Your test code that logs
    log.Println("important message")
    
    // Check captured output
    captured := buffer.String()
    if !strings.Contains(captured, "important message") {
        t.Errorf("Expected captured log to contain message")
    }
}
```

## Implementation Details

The test logging system works by:

1. **Intercepting log output**: Replaces the default log writer with a custom writer
2. **Buffering**: Accumulates partial lines until a complete line is available
3. **Forwarding**: Sends complete lines to `t.Logf` with proper formatting
4. **Cleanup**: Restores original log settings after the test completes

## Benefits

- **Test isolation**: Log output is associated with the correct test
- **Parallel testing**: Each test's logs are kept separate
- **Debugging**: When tests fail, relevant logs are immediately visible
- **CI/CD friendly**: Works well with test runners and CI systems

## Best Practices

1. Always use `defer cleanup()` to ensure proper cleanup
2. Add logging setup at the start of test functions, not in init()
3. For subtests, each can have its own logging setup:
   ```go
   t.Run("Subtest", func(t *testing.T) {
       cleanup := SetupTestLogging(t)
       defer cleanup()
       // subtest code
   })
   ```

## Files

- `test_helpers.go`: Main package test helpers
- `test_helpers_test.go`: Tests for the helpers
- `test_helpers_external_test.go`: External package test helpers
- `api/test_helpers.go`: API package test helpers

## Example Output

When running tests with `-v` flag:
```
=== RUN   TestModelInit
    test_helpers.go:41: 2025/06/08 13:49:02 aistudio.go:123: Model initialized
    test_helpers.go:41: 2025/06/08 13:49:02 aistudio.go:456: Connection established
--- PASS: TestModelInit (0.05s)
```

Without proper test logging, these messages would go to stderr and not be associated with the test.