package main

import "encoding/json"

// readPortState returns a consolidated dump of the peripheral state that has
// clean accessors (border, ULA clock, FDC status/track/sector, joystick).
func (w *worker) readPortState(raw json.RawMessage) (any, error) {
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeParams(raw, &p); err != nil {
		return nil, err
	}
	e, err := w.getMachine(p.Name)
	if err != nil {
		return nil, err
	}

	state := map[string]any{
		"border":    e.ULA().BorderColor(),
		"ula_clock": e.ULA().Clock(),
	}
	if bd := e.BetaDisk(); bd != nil {
		state["fdc_status"] = bd.GetStatus()
		state["fdc_track"] = bd.GetTrack()
		state["fdc_sector"] = bd.GetSector()
		state["fdc_intrq"] = bd.GetInterruptRequest()
	}
	if k := e.Kempston(); k != nil {
		state["kempston"] = k.Read(0x1F)
	}
	return state, nil
}
