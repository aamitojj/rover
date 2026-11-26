package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestJivEvalLightModel runs the task eval through the real offline brain
// (no templates: every plan comes from the model). It only runs when a model
// and a llama-server are given:
//
//	ROVER_EVAL_GGUF=/path/Qwen3-1.7B-Q4_K_M.gguf ROVER_EVAL_LLAMA=/path/llama-server \
//	  go test -run JivEvalLightModel -v -timeout 60m .
//
// Questions are answered from the built-in KB either way (the model writes
// the wording, retrieval picks the article), so their score is the no-model one.
func TestJivEvalLightModel(t *testing.T) {
	gguf, server := envOr("EVAL_GGUF"), envOr("EVAL_LLAMA")
	if gguf == "" || server == "" {
		t.Skip("set ROVER_EVAL_GGUF and ROVER_EVAL_LLAMA to run the Light model eval")
	}
	evalFixture(t)
	t.Setenv("ROVER_BRAIN_DIR", t.TempDir())
	_ = withConfig(func(c *Config) error {
		c.Jiv = &JivSettings{Model: "qwen3-1.7b", ModelPath: gguf, Runtime: filepath.Dir(server), Lang: "auto", IdleMin: 20}
		return nil
	})
	t.Cleanup(brainSidecar.Stop)
	_, ts := loadEval(t)
	c := snapshotConfig()
	type row struct {
		Q, Want, Got string
		OK, Hybrid   bool
		Secs         float64
	}
	var rows []row
	ok, hyb := 0, 0
	for _, x := range ts {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		start := time.Now()
		got := ""
		d, err := planFromModel(ctx, &c, x.Q, x.Site)
		if err == nil {
			var p *Plan
			if p, err = validatePlan(&c, x.Q, d, "model"); err == nil {
				got = strings.Join(tools(p), ",")
			}
		}
		if err != nil {
			got = "error: " + err.Error()
		}
		cancel()
		want := strings.Join(x.Want, ",")
		r := row{Q: x.Q, Want: want, Got: got, OK: got == want, Secs: time.Since(start).Seconds()}
		// what users get: templates first, the model only when they find nothing
		if p, _, err := makePlan(context.Background(), &c, x.Q, x.Site, false); err == nil && strings.Join(tools(p), ",") == want {
			r.Hybrid = true
			hyb++
		}
		if r.OK {
			ok++
		}
		t.Logf("%-4v %5.1fs %q -> %s (want %s)", r.OK, r.Secs, x.Q, got, want)
		rows = append(rows, r)
	}
	t.Logf("Light model (Qwen3-1.7B, CPU): model-only plans %d/%d; shipped planner (templates + model) %d/%d", ok, len(ts), hyb, len(ts))
	if out := envOr("EVAL_OUT"); out != "" {
		b, _ := json.MarshalIndent(map[string]any{"mode": "light model only", "model": filepath.Base(gguf), "tasks_ok": ok, "tasks": len(ts), "hybrid_ok": hyb, "rows": rows}, "", " ")
		_ = os.WriteFile(out, b, 0o644)
	}
}
