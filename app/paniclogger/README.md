# Panic Logger

A dedicated package for logging panic events to a local file. This ensures that all panic events are preserved in a separate log file for debugging and analysis, regardless of other logging configurations.

## Features

- **Dedicated Panic Logging**: All panic events are written to `panic.log` in the `HYDRAIDE_ROOT_PATH/logs/` directory
- **Always Active**: Panic logs are written even if Graylog or other logging systems are unavailable
- **Automatic File Rotation**: Log files are rotated when they exceed 50MB to prevent unbounded growth
- **Thread-Safe**: Safe for concurrent use across multiple goroutines
- **Formatted Output**: Clear, structured panic log entries with timestamp, context, error, and stack trace

## Usage

### Initialization

Initialize the panic logger at application startup:

```go
import "github.com/hydraide/hydraide/app/paniclogger"

func main() {
    // Initialize panic logger
    if err := paniclogger.Init(); err != nil {
        fmt.Printf("WARNING: failed to initialize panic logger: %v\n", err)
    }
    defer func() {
        if err := paniclogger.Close(); err != nil {
            fmt.Printf("WARNING: failed to close panic logger: %v\n", err)
        }
    }()
    
    // ... rest of your application
}
```

### Logging Panics

The panic logger is typically used in conjunction with the `panichandler` package:

```go
import (
    "github.com/hydraide/hydraide/app/panichandler"
    "github.com/hydraide/hydraide/app/paniclogger"
)

func main() {
    defer panichandler.Recover("main function")
    
    // Your application code
}
```

The `panichandler` package automatically calls `paniclogger.LogPanic()` when a panic is recovered.

### Direct Usage

You can also log panics directly (though using `panichandler` is recommended):

```go
if r := recover(); r != nil {
    stackTrace := debug.Stack()
    paniclogger.LogPanic("my context", r, string(stackTrace))
}
```

## Log File Format

Panic logs are written in a clear, structured format:

```
================================================================================
PANIC DETECTED
================================================================================
Timestamp: 2024-11-05T10:30:45.123+01:00
Context:   main function
Error:     runtime error: invalid memory address or nil pointer dereference

Stack Trace:
goroutine 1 [running]:
main.main()
    /app/main.go:42 +0x123
...
================================================================================
```

## File Rotation

- Log files are automatically rotated when they exceed 50MB
- Old logs are saved as `panic.log.old`
- Only one backup file is kept to prevent excessive disk usage

## Configuration

The panic logger uses the `HYDRAIDE_ROOT_PATH` environment variable to determine where to store log files:

- Default path: `/hydraide/logs/panic.log`
- Custom path: `$HYDRAIDE_ROOT_PATH/logs/panic.log`

## Integration with Other Logging

The panic logger operates independently from the standard application logging (console, Graylog, etc.):

- **Console/Graylog**: Regular application logs
- **panic.log**: Panic events only (always enabled)
- **fallback.log**: Optional fallback for regular logs when Graylog is unavailable (controlled by `LOCAL_LOGGING_ENABLED` env var)

This separation ensures that critical panic information is never lost, even if other logging systems fail.

## Best Practices

1. **Always initialize**: Call `paniclogger.Init()` at application startup
2. **Always close**: Defer `paniclogger.Close()` to ensure logs are flushed
3. **Use with panichandler**: Combine with the `panichandler` package for comprehensive panic recovery
4. **Monitor the file**: Set up monitoring for `panic.log` - its presence indicates application issues
5. **Review regularly**: Panic logs should be rare; investigate any occurrences immediately
