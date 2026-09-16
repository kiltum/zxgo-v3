package main

import "encoding/json"

// setTrace turns the delta trace on or off for a machine.
func (w *worker) setTrace(raw json.RawMessage) (any, error) {
	var p struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	e.SetTraceEnabled(p.Enabled)
	return "ok", nil
}

// traceGet returns up to n most-recent trace records for a machine.
func (w *worker) traceGet(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
		N    int    `json:"n"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}
	return e.TraceGet(p.N), nil
}

// traceDiff compares the last n trace records of two machines and returns the
// first divergence, or "identical" if they match.
func (w *worker) traceDiff(raw json.RawMessage) (any, error) {
	var p struct {
		NameA string `json:"name_a"`
		NameB string `json:"name_b"`
		N     int    `json:"n"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	ea, err := w.getMachine(p.NameA)
	if err != nil {
		return nil, err
	}
	eb, err := w.getMachine(p.NameB)
	if err != nil {
		return nil, err
	}

	ta := ea.TraceGet(p.N)
	tb := eb.TraceGet(p.N)
	n := len(ta)
	if len(tb) < n {
		n = len(tb)
	}
	for i := 0; i < n; i++ {
		ja, _ := json.Marshal(ta[i])
		jb, _ := json.Marshal(tb[i])
		if string(ja) != string(jb) {
			return map[string]any{
				"diverged": true,
				"index":    i,
				"a":        ta[i],
				"b":        tb[i],
			}, nil
		}
	}
	return map[string]any{"diverged": false, "compared": n}, nil
}
