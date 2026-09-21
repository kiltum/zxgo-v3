package frontend

// zxKeyboard is what is printed on each key of a ZX Spectrum: the key itself, what
// CAPS SHIFT makes of it, what SYMBOL SHIFT makes of it, and the keyword or keywords
// the two shifts together give.
//
// It is data because three things need the same answers and must not disagree: the
// on-screen keyboard, which draws the legends; the bindings editor, which names the key
// a host key is bound to; and a user reading either. The legends come from
// docs/Sinclair ZX Spectrum keyboard layout.html - the reference this repository keeps -
// rather than from memory, the fourth legend in particular being nothing to guess at
// (CAPS SHIFT plus SYMBOL SHIFT plus a key gives two keywords for most of the letters
// and one for the digits). Two of them, the pound sign and the copyright sign, are
// written as escapes: no source file in this tree carries a non-ASCII character.
type ZXKey struct {
	Row, Col int
	// Key is the key's own legend, Caps what CAPS SHIFT gives it, Symbol what SYMBOL
	// SHIFT gives it, and Extended the keyword or keywords both shifts give together.
	Key      string
	Caps     string
	Symbol   string
	Extended string
}

// ZXKeyboardLen is how many keys the table has: the matrix is 8 half-rows of 5, and
// every cell of it is a key.
const ZXKeyboardLen = 40

// zxKeyboard is one entry per cell, in row order.
var zxKeyboard = [ZXKeyboardLen]ZXKey{
	// Half-row 0.
	{0, 0, "CAPS SHIFT", "", "", ""},
	{0, 1, "Z", "COPY", ":", "LN BEEP"},
	{0, 2, "X", "CLEAR", "\u00a3", "EXP INK"},
	{0, 3, "C", "CONT", "?", "LPRINT PAPER"},
	{0, 4, "V", "CLS", "/", "LLIST FLASH"},
	// Half-row 1.
	{1, 0, "A", "NEW", "STOP", "READ ~"},
	{1, 1, "S", "SAVE", "NOT", "RESTORE |"},
	{1, 2, "D", "DIM", "STEP", "DATA \\"},
	{1, 3, "F", "FOR", "TO", "SGN {"},
	{1, 4, "G", "GOTO", "THEN", "ABS }"},
	// Half-row 2.
	{2, 0, "Q", "PLOT", "<=", "SIN ASN"},
	{2, 1, "W", "DRAW", "<>", "COS ACS"},
	{2, 2, "E", "REM", ">=", "TAN ATN"},
	{2, 3, "R", "RUN", "<", "INT VERIFY"},
	{2, 4, "T", "RAND", ">", "RND MERGE"},
	// Half-row 3.
	{3, 0, "1", "EDIT", "!", "DEF FN"},
	{3, 1, "2", "CAPS LOCK", "@", "FN"},
	{3, 2, "3", "TRUE VID", "#", "LINE"},
	{3, 3, "4", "INV VID", "$", "OPEN #"},
	{3, 4, "5", "<-", "%", "CLOSE #"},
	// Half-row 4.
	{4, 0, "0", "DELETE", "_", "FORMAT"},
	{4, 1, "9", "GRAPHICS", ")", "CAT"},
	{4, 2, "8", "->", "(", "POINT"},
	{4, 3, "7", "^", "'", "ERASE"},
	{4, 4, "6", "v", "&", "MOVE"},
	// Half-row 5.
	{5, 0, "P", "PRINT", "\"", "TAB \u00a9"},
	{5, 1, "O", "POKE", ";", "PEEK OUT"},
	{5, 2, "I", "INPUT", "AT", "CODE IN"},
	{5, 3, "U", "IF", "OR", "CHR$ ]"},
	{5, 4, "Y", "RETURN", "AND", "STR$ ["},
	// Half-row 6.
	{6, 0, "ENTER", "", "", ""},
	{6, 1, "L", "LET", "=", "USR ATTR"},
	{6, 2, "K", "LIST", "+", "LEN SCREEN$"},
	{6, 3, "J", "LOAD", "-", "VAL VAL$"},
	{6, 4, "H", "GOSUB", "^", "SQR CIRCLE"},
	// Half-row 7.
	{7, 0, "SPACE", "BREAK", "", ""},
	{7, 1, "SYMBOL SHIFT", "", "", ""},
	{7, 2, "M", "PAUSE", ".", "PI INVERSE"},
	{7, 3, "N", "NEXT", ",", "INKEY$ OVER"},
	{7, 4, "B", "BORDER", "*", "BIN BRIGHT"},
}

// ZXKeyAt returns what is printed on a key of the matrix, and whether the cell is a key
// at all. Every cell is one, but a caller that indexes by a number it computed should
// not have to assume it.
func ZXKeyAt(row, col int) (ZXKey, bool) {
	for _, k := range zxKeyboard {
		if k.Row == row && k.Col == col {
			return k, true
		}
	}
	return ZXKey{}, false
}

// zxKeyboardRows is the matrix cell of each key as it is laid out on the machine: four
// rows of ten - the numbers as printed, then Q, A and Z - which is how anyone looking at
// a ZX Spectrum sees it.
//
// The table above is in *matrix* order, which is how a key press is described (half-row
// and bit) and not how a keyboard is drawn. The two orders are different on purpose and
// neither can be derived from the other: the number keys run 1 to 0 left to right on the
// machine and 1-5 then 0-9 reversed in the matrix, and the Q row continues into the P
// row's half-rows.
var zxKeyboardRows = [4][10]Cell{
	{{3, 0}, {3, 1}, {3, 2}, {3, 3}, {3, 4}, {4, 4}, {4, 3}, {4, 2}, {4, 1}, {4, 0}},
	{{2, 0}, {2, 1}, {2, 2}, {2, 3}, {2, 4}, {5, 4}, {5, 3}, {5, 2}, {5, 1}, {5, 0}},
	{{1, 0}, {1, 1}, {1, 2}, {1, 3}, {1, 4}, {6, 4}, {6, 3}, {6, 2}, {6, 1}, {6, 0}},
	{{0, 0}, {0, 1}, {0, 2}, {0, 3}, {0, 4}, {7, 4}, {7, 3}, {7, 2}, {7, 1}, {7, 0}},
}

// KeyboardRows is the keyboard in the order it is drawn, which is the order this file's
// table is not in. It is a function rather than the variable so that a caller cannot
// write to it.
func KeyboardRows() [4][10]Cell { return zxKeyboardRows }
