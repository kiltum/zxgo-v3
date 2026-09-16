package cpu

// indexReg abstracts over IX and IY for DD/FD prefix handling.
// This eliminates the ~1000 lines of duplicated opcode code present
// in both zxgo and zxcpp.
type indexReg interface {
	Full() uint16
	SetFull(uint16)
	High() uint8
	Low() uint8
	SetHigh(uint8)
	SetLow(uint8)
}

// ixRef adapts IX to the indexReg interface.
type ixRef struct{ z *Z80 }

func (r ixRef) Full() uint16     { return r.z.IX }
func (r ixRef) SetFull(v uint16) { r.z.IX = v }
func (r ixRef) High() uint8      { return r.z.getIXH() }
func (r ixRef) Low() uint8       { return r.z.getIXL() }
func (r ixRef) SetHigh(v uint8)  { r.z.setIXH(v) }
func (r ixRef) SetLow(v uint8)   { r.z.setIXL(v) }

// iyRef adapts IY to the indexReg interface.
type iyRef struct{ z *Z80 }

func (r iyRef) Full() uint16     { return r.z.IY }
func (r iyRef) SetFull(v uint16) { r.z.IY = v }
func (r iyRef) High() uint8      { return r.z.getIYH() }
func (r iyRef) Low() uint8       { return r.z.getIYL() }
func (r iyRef) SetHigh(v uint8)  { r.z.setIYH(v) }
func (r iyRef) SetLow(v uint8)   { r.z.setIYL(v) }
