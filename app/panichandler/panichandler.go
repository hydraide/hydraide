package panichandler

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/hydraide/hydraide/app/paniclogger"
)

// PanicHandler kezeli a pánikot egyszerű formában (backward compatibility)
// Használat: defer panichandler.PanicHandler()
func PanicHandler() {
	if r := recover(); r != nil {
		stackTrace := debug.Stack()

		// Mindig írjuk a panic.log fájlba
		paniclogger.LogPanic("unknown context", r, string(stackTrace))

		slog.Error("caught panic",
			slog.Any("error", r),
			slog.String("stack", string(stackTrace)),
		)
	}
}

// Recover kezeli a pánikot és logol stack trace-szel
// Használat: defer panichandler.Recover("kontextus leírás")
func Recover(context string) {
	if r := recover(); r != nil {
		stackTrace := debug.Stack()

		// Mindig írjuk a panic.log fájlba
		paniclogger.LogPanic(context, r, string(stackTrace))

		slog.Error("caught panic",
			slog.String("context", context),
			slog.Any("error", r),
			slog.String("stack", string(stackTrace)),
		)
	}
}

// RecoverWithCallback kezeli a pánikot és végrehajt egy callback függvényt pánik esetén
// Használat: defer panichandler.RecoverWithCallback("kontextus", func() { ... })
func RecoverWithCallback(context string, callback func()) {
	if r := recover(); r != nil {
		stackTrace := debug.Stack()

		// Mindig írjuk a panic.log fájlba
		paniclogger.LogPanic(context, r, string(stackTrace))

		slog.Error("caught panic",
			slog.String("context", context),
			slog.Any("error", r),
			slog.String("stack", string(stackTrace)),
		)
		if callback != nil {
			callback()
		}
	}
}

// RecoverWithData kezeli a pánikot és logol extra adatokkal
// Használat: defer panichandler.RecoverWithData("kontextus", map[string]any{"key": "value"})
func RecoverWithData(context string, data map[string]any) {
	if r := recover(); r != nil {
		stackTrace := debug.Stack()

		// Mindig írjuk a panic.log fájlba
		paniclogger.LogPanic(context, r, string(stackTrace))

		attrs := []any{
			slog.String("context", context),
			slog.Any("error", r),
			slog.String("stack", string(stackTrace)),
		}

		// Extra adatok hozzáadása
		for key, value := range data {
			attrs = append(attrs, slog.Any(key, value))
		}

		slog.Error("caught panic", attrs...)
	}
}

// SafeGo biztonságosan indít egy goroutine-t panic kezeléssel
// A goroutine panic esetén NEM lövi ki az egész alkalmazást, csak logolja a hibát
// Használat: panichandler.SafeGo("goroutine kontextus", func() { ... })
func SafeGo(context string, fn func()) {
	go func() {
		defer recoverGoroutine(fmt.Sprintf("goroutine: %s", context), nil)
		fn()
	}()
}

// SafeGoWithCallback biztonságosan indít egy goroutine-t panic kezeléssel és callback-kel
// A callback CSAK AKKOR fut le, ha panic történt (nem lövi ki az appot)
// Használat: panichandler.SafeGoWithCallback("goroutine kontextus", func() { ... }, func() { ... })
func SafeGoWithCallback(context string, fn func(), callback func()) {
	go func() {
		defer recoverGoroutine(fmt.Sprintf("goroutine: %s", context), callback)
		fn()
	}()
}

// SafeGoWithData biztonságosan indít egy goroutine-t panic kezeléssel és extra adatokkal
// Használat: panichandler.SafeGoWithData("goroutine kontextus", map[string]any{"key": "value"}, func() { ... })
func SafeGoWithData(context string, data map[string]any, fn func()) {
	go func() {
		defer recoverGoroutineWithData(fmt.Sprintf("goroutine: %s", context), data, nil)
		fn()
	}()
}

// SafeGoWithDataAndCallback biztonságosan indít egy goroutine-t panic kezeléssel, extra adatokkal és callback-kel
// Használat: panichandler.SafeGoWithDataAndCallback("goroutine kontextus", map[string]any{"key": "value"}, func() { ... }, func() { ... })
func SafeGoWithDataAndCallback(context string, data map[string]any, fn func(), callback func()) {
	go func() {
		defer recoverGoroutineWithData(fmt.Sprintf("goroutine: %s", context), data, callback)
		fn()
	}()
}

// recoverGoroutine belső függvény a goroutine panic kezelésére
// A panic NEM lövi ki az alkalmazást, csak logolja
func recoverGoroutine(context string, callback func()) {
	if r := recover(); r != nil {
		stackTrace := debug.Stack()

		// Mindig írjuk a panic.log fájlba
		paniclogger.LogPanic(context, r, string(stackTrace))

		// Logoljuk slog-ba is
		slog.Error("goroutine panic caught (app continues running)",
			slog.String("context", context),
			slog.Any("error", r),
			slog.String("stack", string(stackTrace)),
		)

		// Opcionális callback végrehajtása (pl. cleanup, értesítés)
		if callback != nil {
			// Védelem a callback panic ellen is
			defer func() {
				if r2 := recover(); r2 != nil {
					slog.Error("panic in goroutine panic callback",
						slog.String("original_context", context),
						slog.Any("callback_error", r2),
					)
				}
			}()
			callback()
		}
	}
}

// recoverGoroutineWithData belső függvény a goroutine panic kezelésére extra adatokkal
func recoverGoroutineWithData(context string, data map[string]any, callback func()) {
	if r := recover(); r != nil {
		stackTrace := debug.Stack()

		// Mindig írjuk a panic.log fájlba
		paniclogger.LogPanic(context, r, string(stackTrace))

		// Logoljuk slog-ba is extra adatokkal
		attrs := []any{
			slog.String("context", context),
			slog.Any("error", r),
			slog.String("stack", string(stackTrace)),
		}

		// Extra adatok hozzáadása
		for key, value := range data {
			attrs = append(attrs, slog.Any(key, value))
		}

		slog.Error("goroutine panic caught with data (app continues running)", attrs...)

		// Opcionális callback végrehajtása
		if callback != nil {
			defer func() {
				if r2 := recover(); r2 != nil {
					slog.Error("panic in goroutine panic callback",
						slog.String("original_context", context),
						slog.Any("callback_error", r2),
					)
				}
			}()
			callback()
		}
	}
}
