package client

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// TestTraceContractDecode is the cross-contract guard: testdata/trace_contract.json has its keys copied
// from the replypen server's clidebug trace structs (handlers.go / trace.go) and exercises every nullable
// field on BOTH sides (a present blob/value AND an explicit null). It asserts the CLI's Trace decodes the
// payload WITHOUT dropping any field — DisallowUnknownFields makes a server key the CLI struct lacks a hard
// failure, so a future server-side field addition can't silently vanish from the table/decompose paths.
func TestTraceContractDecode(t *testing.T) {
	raw, err := os.ReadFile("testdata/trace_contract.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields() // a server key with no CLI struct field → error (dropped-field guard)
	var tr Trace
	if err := dec.Decode(&tr); err != nil {
		t.Fatalf("decode trace (a server field is likely missing from types.go): %v", err)
	}

	// Spot-check the load-bearing, easy-to-misType fields actually round-tripped.
	if tr.Thread.ProcessorFailure == nil {
		t.Error("thread.processor_failure dropped — it is a JSON blob, must be json.RawMessage")
	}
	if tr.Thread.PipelineStartedAt == nil || *tr.Thread.PipelineStartedAt == "" {
		t.Error("thread.pipeline_started_at dropped")
	}
	if len(tr.Drafts) != 2 {
		t.Fatalf("drafts: got %d want 2", len(tr.Drafts))
	}
	if tr.Drafts[0].ExternalDraftID == nil || *tr.Drafts[0].ExternalDraftID != "r-998877" {
		t.Error("drafts[0].external_draft_id dropped")
	}
	if tr.Drafts[1].PlacedAt != nil {
		t.Error("drafts[1].placed_at should be nil for a null")
	}

	// Nullable numerics: a present value and an explicit null must be distinguishable.
	if len(tr.Logs) != 2 {
		t.Fatalf("logs: got %d want 2", len(tr.Logs))
	}
	if tr.Logs[0].DurationMs == nil || *tr.Logs[0].DurationMs != 1200 {
		t.Error("logs[0].duration_ms should decode to 1200")
	}
	if tr.Logs[1].DurationMs != nil {
		t.Error("logs[1].duration_ms should be nil for a null (not silently 0)")
	}
	if len(tr.Deliveries) != 2 {
		t.Fatalf("deliveries: got %d want 2", len(tr.Deliveries))
	}
	if tr.Deliveries[0].ResponseStatus == nil || *tr.Deliveries[0].ResponseStatus != 200 {
		t.Error("deliveries[0].response_status should decode to 200")
	}
	if tr.Deliveries[1].ResponseStatus != nil {
		t.Error("deliveries[1].response_status should be nil for a null (not silently 0)")
	}
}
