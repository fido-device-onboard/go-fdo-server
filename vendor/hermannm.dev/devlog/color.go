package devlog

type color []byte

// ANSI color codes (https://en.wikipedia.org/wiki/ANSI_escape_code#Colors).
var (
	colorReset   = color("\x1b[0m")
	colorRed     = color("\x1b[31m")
	colorGreen   = color("\x1b[32m")
	colorYellow  = color("\x1b[33m")
	colorMagenta = color("\x1b[35m")
	colorCyan    = color("\x1b[36m")
	colorGray    = color("\x1b[37m")
	noColor      = color{}
)

func (handler *Handler) setColor(buffer *byteBuffer, color color) {
	if handler.options.DisableColors {
		return
	}

	buffer.Write(color)
}

func (handler *Handler) resetColor(buffer *byteBuffer) {
	handler.setColor(buffer, colorReset)
}

func (handler *Handler) writeByteWithColor(buffer *byteBuffer, byte byte, color color) {
	handler.setColor(buffer, color)
	buffer.writeByte(byte)
	handler.resetColor(buffer)
}
