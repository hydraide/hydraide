# HydrAIDE Logging Architecture Refactoring

## Overview

This document describes the refactored logging architecture for HydrAIDE, which addresses the issues with excessive local file logging and provides better panic handling for goroutines.

## Problems Solved

### 1. Excessive Local File Logging
**Problem**: The previous implementation created large log files (500MB+) even when Graylog was enabled, wasting disk space.

**Solution**: 
- Local file logging (`fallback.log`) is now **optional** and controlled by the `LOCAL_LOGGING_ENABLED` environment variable
- Default behavior: logs go to console and/or Graylog only
- Panic logs are separated into a dedicated `panic.log` file (always enabled)

### 2. Unprotected Goroutines
**Problem**: Goroutines were not protected against panics, which could crash the entire application.

**Solution**:
- Enhanced `panichandler` package with `SafeGo()` family of functions
- Goroutines can now be launched with automatic panic recovery
- Panics in goroutines are logged but don't crash the application

## New Architecture

### Logging Layers

```
┌─────────────────────────────────────────────────────────────┐
│                    Application Logs                          │
└─────────────────────────────────────────────────────────────┘
                            │
                ┌───────────┴───────────┐
                │                       │
         ┌──────▼──────┐        ┌──────▼──────┐
         │   Console   │        │   Graylog   │
         │  (ALWAYS)   │        │ (OPTIONAL)  │
         └─────────────┘        └─────┬───────┘
                                      │
                         ┌────────────▼────────────┐
                         │   fallback.log          │
                         │ (ONLY if enabled AND    │
                         │  Graylog unavailable)   │
                         └─────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│                      Panic Logs                              │
│                   (Separate System)                          │
└─────────────────────────────────────────────────────────────┘
                            │
                     ┌──────▼──────┐
                     │  panic.log  │
                     │  (ALWAYS)   │
                     └─────────────┘
```

### Environment Variables

#### `LOCAL_LOGGING_ENABLED` (NEW)
- **Default**: `false`
- **Values**: `true` | `false`
- **Purpose**: Enables local file logging of all application logs
- **When to use**: 
  - Development/debugging when Graylog is not available
  - Need to retain logs locally for compliance/audit purposes
  - Troubleshooting Graylog connectivity issues

**Warning**: Enabling this can create large log files quickly. Use only when necessary.

## File Structure

### Log Files Location

All log files are stored in `$HYDRAIDE_ROOT_PATH/logs/`:

```
/hydraide/logs/
├── panic.log       # Panic events only (always enabled, 50MB max)
├── panic.log.old   # Rotated panic log backup
├── fallback.log    # Application logs (only if LOCAL_LOGGING_ENABLED=true, 10MB max)
└── fallback.log.old # Rotated fallback log backup
```

### Log File Purposes

| File | Purpose | Always Enabled | Max Size | Rotation |
|------|---------|----------------|----------|----------|
| `panic.log` | Critical panic events | ✅ Yes | 50MB | Yes |
| `fallback.log` | Application logs backup | ❌ No* | 10MB | Yes |

\* Only when `LOCAL_LOGGING_ENABLED=true` AND Graylog is unavailable

## Components

### 1. paniclogger Package

New package located at `app/paniclogger/`

**Features**:
- Dedicated panic log file management
- Always active (independent of other logging config)
- Thread-safe concurrent writes
- Automatic file rotation at 50MB
- Structured panic log format with timestamp, context, error, and stack trace

**Usage**:
```go
// Initialize at startup
if err := paniclogger.Init(); err != nil {
    fmt.Printf("WARNING: failed to initialize panic logger: %v\n", err)
}
defer paniclogger.Close()

// Automatically called by panichandler
paniclogger.LogPanic("context", panicError, stackTrace)
```

### 2. panichandler Package (Enhanced)

Location: `app/panichandler/`

**New Functions**:
- `SafeGo(context, fn)` - Launch goroutine with panic protection
- `SafeGoWithCallback(context, fn, callback)` - With callback on panic
- `SafeGoWithData(context, data, fn)` - With extra logging data
- `SafeGoWithDataAndCallback(...)` - Combined features

**Key Changes**:
- Removed external dependencies (pushover, trendizz-api)
- Uses local `paniclogger` package
- All panic recovery functions now log to `panic.log`

**Usage**:
```go
// Protect main function
defer panichandler.Recover("main function")

// Launch safe goroutine
panichandler.SafeGo("worker goroutine", func() {
    // Your code here
    // Panics will be caught and logged, app continues running
})

// With cleanup callback
panichandler.SafeGoWithCallback("worker", func() {
    // Work...
}, func() {
    // Cleanup after panic
})
```

### 3. main.go Updates

**Initialization Order**:
```go
func main() {
    defer panicHandler()
    
    // 1. Initialize panic logger (FIRST)
    paniclogger.Init()
    defer paniclogger.Close()
    
    // 2. Initialize application logging
    // Console (always) + optional Graylog + optional fallback
    
    // 3. Start server
    // ...
}
```

**Panic Handler**:
```go
func panicHandler() {
    if r := recover(); r != nil {
        stackTrace := debug.Stack()
        
        // Log to panic.log
        paniclogger.LogPanic("main function", r, string(stackTrace))
        
        // Log to console/Graylog
        slog.Error("caught panic", "error", r, "stack", string(stackTrace))
        
        gracefulStop()
    }
}
```

## Configuration Examples

### Production (Minimal Disk Usage)
```bash
LOG_LEVEL=info
LOCAL_LOGGING_ENABLED=false  # No local files except panic.log
GRAYLOG_ENABLED=true
GRAYLOG_SERVER=graylog.example.com:12201
```

**Result**:
- Console: ✅ All logs
- Graylog: ✅ All logs
- fallback.log: ❌ Not created
- panic.log: ✅ Only if panic occurs

### Development (Full Local Logging)
```bash
LOG_LEVEL=debug
LOCAL_LOGGING_ENABLED=true   # Enable local file logging
GRAYLOG_ENABLED=false
```

**Result**:
- Console: ✅ All logs
- Graylog: ❌ Disabled
- fallback.log: ❌ Not created (Graylog not configured)
- panic.log: ✅ Only if panic occurs

### Hybrid (Graylog + Local Backup)
```bash
LOG_LEVEL=info
LOCAL_LOGGING_ENABLED=true   # Enable fallback when Graylog is down
GRAYLOG_ENABLED=true
GRAYLOG_SERVER=graylog.example.com:12201
```

**Result**:
- Console: ✅ All logs
- Graylog: ✅ All logs (when available)
- fallback.log: ✅ Created when Graylog is unavailable
- panic.log: ✅ Only if panic occurs

## Migration Guide

### For Existing Deployments

1. **Update .env file** (optional):
   ```bash
   # Add this line if you want local file logging
   LOCAL_LOGGING_ENABLED=false  # Default, no change in behavior
   ```

2. **Review disk space**:
   - Old `fallback.log` files can be deleted if no longer needed
   - `panic.log` is much smaller (only panic events)

3. **Update monitoring**:
   - Monitor `panic.log` file existence (indicates problems)
   - Remove monitoring for `fallback.log` if `LOCAL_LOGGING_ENABLED=false`

### For New Deployments

1. Use `.env_sample` as template
2. Set `LOCAL_LOGGING_ENABLED=false` for production
3. Configure Graylog if available
4. Set up alerts for `panic.log` file creation

## Benefits

### Resource Efficiency
- **Disk Space**: Reduced from 500MB+ to ~0MB for regular logs (when LOCAL_LOGGING_ENABLED=false)
- **I/O**: Less disk write operations
- **Performance**: No performance impact from file logging

### Reliability
- **Panic Logs Always Saved**: Critical errors never lost
- **Goroutine Safety**: Application continues running even when goroutines panic
- **Separated Concerns**: Panic logs separate from application logs

### Flexibility
- **User Control**: Admins can enable local logging if needed
- **No Breaking Changes**: Default behavior maintains system stability
- **Easy Debugging**: Panic logs in standardized, readable format

## Testing

Comprehensive test coverage for `paniclogger`:
```bash
go test ./app/paniclogger/... -v
```

Tests verify:
- ✅ Initialization and file creation
- ✅ Panic logging functionality
- ✅ Concurrent write safety
- ✅ File rotation at size limits
- ✅ Graceful fallback when not initialized

## Troubleshooting

### Panic logs not being created
1. Check `HYDRAIDE_ROOT_PATH` environment variable
2. Ensure write permissions on logs directory
3. Check initialization: `paniclogger.Init()` must be called

### Want to keep local logs
Set `LOCAL_LOGGING_ENABLED=true` in `.env`

### Disk space concerns
- Panic logs auto-rotate at 50MB
- Fallback logs auto-rotate at 10MB (when enabled)
- Only one backup file is kept for each

### Monitoring panic.log
```bash
# Alert if panic.log exists and is not empty
if [ -s /hydraide/logs/panic.log ]; then
    echo "WARNING: Application panics detected!"
fi
```

## Future Enhancements

Potential improvements:
- Remote panic log shipping
- Panic statistics/metrics
- Configurable panic log rotation size
- Integration with alerting systems
- Structured JSON format for panic logs
