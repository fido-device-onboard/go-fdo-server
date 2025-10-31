package devlog

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

type byteBuffer []byte

// Always returns nil error (still has error in signature, to satisfy [io.Writer] interface).
func (buffer *byteBuffer) Write(bytes []byte) (bytesWritten int, err error) {
	*buffer = append(*buffer, bytes...)
	return len(bytes), nil
}

func (buffer *byteBuffer) writeString(str string) {
	*buffer = append(*buffer, str...)
}

func (buffer *byteBuffer) writeByte(b byte) {
	*buffer = append(*buffer, b)
}

func (buffer *byteBuffer) writeDecimal(decimal int) {
	*buffer = strconv.AppendInt(*buffer, int64(decimal), 10)
}

func (buffer *byteBuffer) writeIndent(indent int) {
	for i := 0; i <= indent; i++ {
		buffer.writeString("  ")
	}
}

func (buffer *byteBuffer) writeAny(value any) {
	*buffer = fmt.Append(*buffer, value)
}

func (buffer *byteBuffer) writeBytesWithIndentedNewlines(bytes []byte, indent int) {
	lastWriteIndex := 0
	for i := 0; i < len(bytes)-1; i++ {
		if bytes[i] == '\n' {
			buffer.Write(bytes[lastWriteIndex : i+1])
			buffer.writeIndent(indent)
			lastWriteIndex = i + 1
		}
	}

	buffer.Write(bytes[lastWriteIndex:])
}

func (buffer *byteBuffer) writeAnyWithIndentedNewlines(value any, indent int) {
	valueBuffer := newSmallBuffer()
	defer valueBuffer.freeSmall()

	valueBuffer.writeAny(value)
	buffer.writeBytesWithIndentedNewlines(*valueBuffer, indent)
}

// Adapted from standard library log package:
// https://github.com/golang/go/blob/ab5bd15941f3cea3695338756d0b8be0ef2321fb/src/log/log.go#L114
func (buffer *byteBuffer) writeTime(t time.Time) {
	hour, min, sec := t.Clock()
	buffer.writeFixedWidthDecimal(hour, 2)
	buffer.writeByte(':')
	buffer.writeFixedWidthDecimal(min, 2)
	buffer.writeByte(':')
	buffer.writeFixedWidthDecimal(sec, 2)
}

// Adapted from standard library log package:
// https://github.com/golang/go/blob/ab5bd15941f3cea3695338756d0b8be0ef2321fb/src/log/log.go#L114
func (buffer *byteBuffer) writeDateTime(t time.Time) {
	year, month, day := t.Date()
	buffer.writeFixedWidthDecimal(year, 4)
	buffer.writeByte('-')
	buffer.writeFixedWidthDecimal(int(month), 2)
	buffer.writeByte('-')
	buffer.writeFixedWidthDecimal(day, 2)
	buffer.writeByte(' ')

	buffer.writeTime(t)
}

// Adapted from standard library log package:
// https://github.com/golang/go/blob/ab5bd15941f3cea3695338756d0b8be0ef2321fb/src/log/log.go#L93
func (buffer *byteBuffer) writeFixedWidthDecimal(decimal int, width int) {
	var bytes [20]byte

	index := len(bytes) - 1
	for decimal >= 10 || width > 1 {
		width--
		remainder := decimal / 10
		bytes[index] = byte('0' + decimal - remainder*10)
		index--
		decimal = remainder
	}

	bytes[index] = byte('0' + decimal)
	*buffer = append(*buffer, bytes[index:]...)
}

func (buffer *byteBuffer) join(other byteBuffer) {
	*buffer = append(*buffer, other...)
}

func (buffer byteBuffer) copy() byteBuffer {
	newBuffer := make(byteBuffer, len(buffer), cap(buffer))
	copy(newBuffer, buffer)
	return newBuffer
}

// Inspired by Jonathan Amsterdam's guide to writing structured log handlers:
// https://github.com/golang/example/blob/1d6d2400d4027025cb8edc86a139c9c581d672f7/slog-handler-guide/README.md#speed
var bufferPool = sync.Pool{
	New: func() any {
		buffer := make(byteBuffer, 0, 1024)
		return &buffer
	},
}

func newBuffer() *byteBuffer {
	return bufferPool.Get().(*byteBuffer)
}

func (buffer *byteBuffer) free() {
	// To reduce peak allocation, return only smaller buffers to the pool.
	const maxBufferSize = 16 * 1024
	if cap(*buffer) <= maxBufferSize {
		*buffer = (*buffer)[:0]
		bufferPool.Put(buffer)
	}
}

var smallBufferPool = sync.Pool{
	New: func() any {
		buffer := make(byteBuffer, 0, 128)
		return &buffer
	},
}

func newSmallBuffer() *byteBuffer {
	return smallBufferPool.Get().(*byteBuffer)
}

func (buffer *byteBuffer) freeSmall() {
	const maxBufferSize = 16 * 128
	if cap(*buffer) <= maxBufferSize {
		*buffer = (*buffer)[:0]
		smallBufferPool.Put(buffer)
	}
}
