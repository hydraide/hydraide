# Panic Handler & Goroutine Safety

This package ensures stable application operation in case of panics.

## 📦 Components

### 1. `paniclogger` - Panic log file management
- **File**: `app/paniclogger/paniclogger.go`
- **Purpose**: All panic events are written to the `panic.log` file
- **Operation**: Thread-safe, immediately flushed (Sync())
- **Always active**: Independent of Graylog

### 2. `panichandler` - Panic handling
- **File**: `app/panichandler/panichandler.go`
- **Purpose**: Catches and handles panics

## 🎯 Two types of panic handling

### A) Defer-based (within functions)
These do **NOT** prevent application shutdown, only log:

- `PanicHandler()` - simple form
- `Recover(context)` - with context
- `RecoverWithCallback(context, callback)` - with callback
- `RecoverWithData(context, data)` - with extra data

**Usage:**
```go
func myFunction() {
    defer panichandler.Recover("myFunction")
    // ... code that might panic
}
```

### B) Goroutine-safe (for launching goroutines)
These **PREVENT** application shutdown:

- `SafeGo(context, fn)` - simple goroutine
- `SafeGoWithCallback(context, fn, callback)` - with callback
- `SafeGoWithData(context, data, fn)` - with extra data
- `SafeGoWithDataAndCallback(context, data, fn, callback)` - both

**Usage:**
```go
// ✅ CORRECT
panichandler.SafeGo("background job", func() {
    // If panic here → app continues running
})

// ❌ WRONG
go func() {
    // If panic here → app crashes!
}()
```

## 🚨 CRITICAL: Main goroutine vs. Worker goroutines

### Main goroutine panic
```go
func main() {
    defer panicHandler() // This is good, gracefully shuts down the app
    // ...
}
```
**Result:** Graceful shutdown, panic.log + slog

### Worker goroutine panic (UNPROTECTED)
```go
go func() {
    // panic here → ENTIRE APP CRASHES ❌
}()
```
**Result:** Immediate app crash, possibly empty panic.log

### Worker goroutine panic (PROTECTED)
```go
panichandler.SafeGo("worker", func() {
    // panic here → app continues running ✅
})
```
**Result:** App continues running, panic.log + slog

## 📊 Logging architecture

```
Panic occurs
    │
    ├─> paniclogger.LogPanic()
    │   └─> writes to panic.log file (ALWAYS)
    │
    └─> slog.Error()
        ├─> Console (ALWAYS)
        └─> Graylog (if available)
```

## 📁 Files

```
panic.log               - Panic events (ALWAYS)
fallback.log           - NO LONGER USED
console output         - ALWAYS
Graylog                - if available
```

## 🔧 API

### paniclogger

```go
// Initialize (once, in main)
paniclogger.Init()

// Log panic (called automatically)
paniclogger.LogPanic(context, panicValue, stackTrace)

// Close (on shutdown)
paniclogger.Close()
```

### panichandler - Defer functions

```go
// Simple
defer panichandler.PanicHandler()
defer panichandler.Recover("context")

// With callback
defer panichandler.RecoverWithCallback("context", func() {
    cleanup()
})

// With extra data
defer panichandler.RecoverWithData("context", map[string]any{
    "user_id": 123,
})
```

### panichandler - Goroutine functions

```go
// Simple
panichandler.SafeGo("job name", func() {
    doWork()
})

// With callback (runs on panic)
panichandler.SafeGoWithCallback("job", func() {
    doWork()
}, func() {
    cleanup() // only on panic
})

// With extra data
panichandler.SafeGoWithData("job", map[string]any{
    "order_id": 456,
}, func() {
    processOrder()
})

// Both
panichandler.SafeGoWithDataAndCallback("job", 
    map[string]any{"id": 789},
    func() { doWork() },
    func() { cleanup() },
)
```

## ✅ Best Practices

1. **In main**: `defer panicHandler()`
2. **In goroutines**: ALWAYS use `SafeGo*()`
3. **In functions**: `defer Recover()` if critical
4. **Context**: Meaningful, debuggable name
5. **Callback**: Only if cleanup is needed
6. **Data**: Only if extra debug info is needed

## 🚫 FORBIDDEN

```go
// ❌ WRONG
go func() {
    doWork()
}()

// ❌ WRONG
go func() {
    defer panichandler.Recover("work")
    doWork()
}()

// ✅ CORRECT
panichandler.SafeGo("work", doWork)
```

## 📖 Detailed Documentation

- [Panic Logger](../paniclogger/README.md)
