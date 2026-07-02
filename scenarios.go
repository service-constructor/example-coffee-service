package main

// Saga demo scenarios. The mini-app picks one; it travels in the quote metadata
// (metadata["scenario"]) and the platform echoes it to /execute, so this service
// can drive every path the orchestrator supports:
//
//	sync-success      /execute -> SUCCESS                        -> COMPLETED
//	sync-fail         /execute -> FAILED                         -> RELEASED (refund)
//	async-success     /execute -> PENDING, webhook SUCCESS       -> COMPLETED
//	async-fail        /execute -> PENDING, webhook FAILED        -> RELEASED (refund)
//	retry-success     /execute -> 503 for N attempts then OK     -> COMPLETED
//	retry-exhausted   /execute -> 503 every attempt              -> RELEASED (retries exhausted)
//	reconcile-done    /execute -> PENDING, no webhook, /status DONE     -> COMPLETED (reconciler)
//	reconcile-notdone /execute -> PENDING, no webhook, /status NOT_DONE -> RELEASED  (reconciler)
//	stuck-unknown     /execute -> PENDING, no webhook, /status UNKNOWN  -> stays PENDING

type scenarioKind string

const (
	kindSync           scenarioKind = "sync"
	kindAsyncCallback  scenarioKind = "async-callback"
	kindAsyncReconcile scenarioKind = "async-reconcile"
	kindRetry          scenarioKind = "retry"
)

type scenario struct {
	ID       string       `json:"id"`
	Title    string       `json:"title"`
	Kind     scenarioKind `json:"-"`
	Verdict  string       `json:"-"` // "SUCCESS" | "FAILED"
	FailN    int          `json:"-"` // retry: leading attempts that return 503 (-1 = always)
	Reconile string       `json:"-"` // async-reconcile: "DONE" | "NOT_DONE" | "UNKNOWN"
}

const defaultScenario = "sync-success"

var scenarios = map[string]scenario{
	"sync-success":      {ID: "sync-success", Title: "Sync · success", Kind: kindSync, Verdict: "SUCCESS"},
	"sync-fail":         {ID: "sync-fail", Title: "Sync · fail (refund)", Kind: kindSync, Verdict: "FAILED"},
	"async-success":     {ID: "async-success", Title: "Async webhook · success", Kind: kindAsyncCallback, Verdict: "SUCCESS"},
	"async-fail":        {ID: "async-fail", Title: "Async webhook · fail (refund)", Kind: kindAsyncCallback, Verdict: "FAILED"},
	"retry-success":     {ID: "retry-success", Title: "Retry then success", Kind: kindRetry, Verdict: "SUCCESS", FailN: 2},
	"retry-exhausted":   {ID: "retry-exhausted", Title: "Retry exhausted (refund)", Kind: kindRetry, Verdict: "FAILED", FailN: -1},
	"reconcile-done":    {ID: "reconcile-done", Title: "Recovery · reconciler DONE", Kind: kindAsyncReconcile, Verdict: "SUCCESS", Reconile: "DONE"},
	"reconcile-notdone": {ID: "reconcile-notdone", Title: "Recovery · reconciler NOT_DONE", Kind: kindAsyncReconcile, Verdict: "FAILED", Reconile: "NOT_DONE"},
	"stuck-unknown":     {ID: "stuck-unknown", Title: "Stuck · status UNKNOWN", Kind: kindAsyncReconcile, Verdict: "FAILED", Reconile: "UNKNOWN"},
}

// scenarioOrder preserves a stable display order for the mini-app picker.
var scenarioOrder = []string{
	"sync-success", "sync-fail",
	"async-success", "async-fail",
	"retry-success", "retry-exhausted",
	"reconcile-done", "reconcile-notdone", "stuck-unknown",
}

func resolveScenario(id string) scenario {
	if s, ok := scenarios[id]; ok {
		return s
	}
	return scenarios[defaultScenario]
}
