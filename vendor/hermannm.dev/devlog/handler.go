package devlog

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"runtime"
	"strings"
	"sync"

	"github.com/neilotoole/jsoncolor"
)

// Handler is a [slog.Handler] that outputs log records in a human-readable format, designed for
// development builds. See the package-level documentation for more on the output format.
type Handler struct {
	output     io.Writer
	outputLock *sync.Mutex
	options    Options

	// Current indent for new attributes, based on the current number of preformatted groups.
	indent                      int
	preformattedAttrs           byteBuffer
	preformattedGroups          byteBuffer
	preformattedGroupsWithAttrs byteBuffer
}

// Options configure a log [Handler].
type Options struct {
	// Level is the minimum log record level that will be logged.
	// If nil, defaults to [slog.LevelInfo].
	Level slog.Leveler

	// AddSource adds a 'source' attribute to every log record, with the file name and line number
	// where the log record was produced.
	// Defaults to false.
	AddSource bool

	// DisableColors removes colors from log output.
	//
	// Colors are enabled by default when the [io.Writer] given to [NewHandler] is a terminal with
	// color support (see [IsColorTerminal]).
	DisableColors bool

	// ForceColors skips checking [IsColorTerminal] for color support, and includes colors in log
	// output regardless. It overrides [Options.DisableColors].
	ForceColors bool

	// TimeFormat controls how time is formatted for each log entry. It defaults to
	// [TimeFormatShort], showing just the time and not the date, but can be set to [TimeFormatFull]
	// to include the date as well.
	TimeFormat TimeFormat
}

// See [Options.TimeFormat].
type TimeFormat int8

const (
	// TimeFormatShort includes just the time, not the date, formatted as: [10:57:30]
	//
	// This is the default time format.
	TimeFormatShort TimeFormat = iota

	// TimeFormatFull includes both date and time, formatted as: [2024-09-29 10:57:30]
	TimeFormatFull
)

// NewHandler creates a log [Handler] that writes to output, using the given options.
// If options is nil, the default options are used.
func NewHandler(output io.Writer, options *Options) *Handler {
	handler := Handler{
		output:                      output,
		outputLock:                  &sync.Mutex{},
		options:                     Options{},
		preformattedAttrs:           nil,
		preformattedGroups:          nil,
		preformattedGroupsWithAttrs: nil,
		indent:                      0,
	}
	if options != nil {
		handler.options = *options
	}

	if handler.options.ForceColors {
		handler.options.DisableColors = false
	} else if !handler.options.DisableColors && !IsColorTerminal(output) {
		handler.options.DisableColors = true
	}

	return &handler
}

// InitDefaultLogHandler is shorthand for:
//   - Calling [NewHandler] with the given arguments
//   - Setting it as the default log handler with [slog.SetDefault]
func InitDefaultLogHandler(output io.Writer, options *Options) {
	handler := NewHandler(output, options)
	slog.SetDefault(slog.New(handler))
}

// Enabled reports whether the handler is configured to log records at the given level.
func (handler *Handler) Enabled(_ context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if handler.options.Level != nil {
		minLevel = handler.options.Level.Level()
	}
	return level >= minLevel
}

// Handle writes the given log record to the handler's output.
// See the package-level documentation for more on the output format.
func (handler *Handler) Handle(_ context.Context, record slog.Record) error {
	buffer := newBuffer()
	defer buffer.free()

	if !record.Time.IsZero() {
		handler.setColor(buffer, colorGray)
		buffer.writeByte('[')

		switch handler.options.TimeFormat {
		case TimeFormatFull:
			buffer.writeDateTime(record.Time)
		case TimeFormatShort:
			fallthrough
		default:
			buffer.writeTime(record.Time)
		}

		buffer.writeByte(']')
		handler.resetColor(buffer)
		buffer.writeByte(' ')
	}

	handler.writeLevel(buffer, record.Level)
	handler.writeByteWithColor(buffer, ':', colorGray)
	buffer.writeByte(' ')

	buffer.writeString(record.Message)
	buffer.writeByte('\n')

	// Preformatted groups that have preformatted attributes: we always want to write these, so that
	// the preformatted attributes written below are shown under their corresponding groups
	buffer.join(handler.preformattedGroupsWithAttrs)

	if record.NumAttrs() > 0 {
		// We only want to write preformattedGroups (without preformatted attrs) if the current
		// record has attributes - otherwise we end up with writing groups with no attributes
		buffer.join(handler.preformattedGroups)

		record.Attrs(func(attr slog.Attr) bool {
			handler.writeAttribute(buffer, attr, handler.indent)
			return true
		})
	}

	// Write preformatted attributes last, so they are shown beneath the current record's attributes
	buffer.join(handler.preformattedAttrs)

	if handler.options.AddSource && record.PC != 0 {
		handler.writeLogSource(buffer, record.PC)
	}

	handler.outputLock.Lock()
	defer handler.outputLock.Unlock()
	_, err := handler.output.Write(*buffer)
	return err
}

// WithAttrs returns a new Handler which adds the given attributes to every log record.
func (handler *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return handler
	}

	// Copies the old handler, but keeps the same mutex since we hold a pointer to it
	newHandler := *handler

	// We want to show newer attributes before old ones, so we write the new ones first before
	// joining the previous ones below
	newHandler.preformattedAttrs = nil
	for _, attr := range attrs {
		newHandler.writeAttribute(&newHandler.preformattedAttrs, attr, newHandler.indent)
	}
	newHandler.preformattedAttrs.join(handler.preformattedAttrs)

	// We want to move previous preformattedGroups to preformattedGroupsWithAttrs, so we always
	// write these groups (since the attributes added here should be displayed under these groups)
	newHandler.preformattedGroupsWithAttrs = handler.preformattedGroups.copy()
	newHandler.preformattedGroups = nil

	return &newHandler
}

// WithGroup returns a new Handler where all future log record attributes are nested under the given
// group name.
func (handler *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return handler
	}

	// Copies the old handler, but keeps the same mutex since we hold a pointer to it
	newHandler := *handler

	// Copy old preformattedGroups so we don't mutate the previous ones
	newHandler.preformattedGroups = handler.preformattedGroups.copy()

	// We then write the new group key to preformattedGroups, and increase the indent on newHandler
	// so future attributes will display under the new group
	newHandler.preformattedGroups.writeIndent(newHandler.indent)
	newHandler.writeAttributeKey(&newHandler.preformattedGroups, name)
	newHandler.preformattedGroups.writeByte('\n')
	newHandler.indent++

	return &newHandler
}

func (handler *Handler) writeLevel(buffer *byteBuffer, level slog.Level) {
	if handler.options.DisableColors {
		buffer.writeString(level.String())
		return
	}

	var levelColor color
	if level >= slog.LevelError {
		levelColor = colorRed
	} else if level >= slog.LevelWarn {
		levelColor = colorYellow
	} else if level >= slog.LevelInfo {
		levelColor = colorGreen
	} else {
		levelColor = colorMagenta
	}

	handler.setColor(buffer, levelColor)
	buffer.writeString(level.String())
	handler.resetColor(buffer)
}

// Interface to allow log input handlers (such as [hermannm.dev/devlog/log]) to pass a log attribute
// value that should be pretty-formatted as JSON by this output handler.
type jsonLogValuer interface {
	JSONLogValue() any
}

func (handler *Handler) writeAttribute(buffer *byteBuffer, attr slog.Attr, indent int) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	buffer.writeIndent(indent)

	switch attr.Value.Kind() {
	case slog.KindGroup:
		attrs := attr.Value.Group()
		if len(attrs) == 0 {
			return
		}

		if attr.Key != "" {
			handler.writeAttributeKey(buffer, attr.Key)
			buffer.writeByte('\n')
			indent++
		}

		for _, groupAttr := range attrs {
			handler.writeAttribute(buffer, groupAttr, indent)
		}
	case slog.KindTime:
		handler.writeAttributeKey(buffer, attr.Key)
		buffer.writeByte(' ')
		buffer.writeDateTime(attr.Value.Time())
		buffer.writeByte('\n')
	case slog.KindAny:
		handler.writeAttributeKey(buffer, attr.Key)

		value := attr.Value.Any()
		if json, ok := value.(jsonLogValuer); ok {
			buffer.writeByte(' ')
			handler.writeJSON(buffer, json.JSONLogValue(), attr.Value, indent)
			return
		}

		reflectValue := reflect.ValueOf(value)
		switch reflectValue.Kind() {
		case reflect.Slice, reflect.Array:
			handler.writeListOrSingleElement(buffer, reflectValue, indent+1)
		default:
			buffer.writeByte(' ')
			buffer.writeString(attr.Value.String())
		}
		buffer.writeByte('\n')
	default:
		handler.writeAttributeKey(buffer, attr.Key)
		buffer.writeByte(' ')
		buffer.writeString(attr.Value.String())
		buffer.writeByte('\n')
	}
}

func (handler *Handler) writeAttributeKey(buffer *byteBuffer, attrKey string) {
	handler.setColor(buffer, colorCyan)
	buffer.writeString(attrKey)
	handler.writeByteWithColor(buffer, ':', colorGray)
}

var jsonColors = jsoncolor.Colors{
	Key:           jsoncolor.Color(colorCyan),
	Punc:          jsoncolor.Color(colorGray),
	String:        jsoncolor.Color(noColor),
	Number:        jsoncolor.Color(noColor),
	Bool:          jsoncolor.Color(noColor),
	Bytes:         jsoncolor.Color(noColor),
	Time:          jsoncolor.Color(noColor),
	Null:          jsoncolor.Color(noColor),
	TextMarshaler: jsoncolor.Color(noColor),
}

func (handler *Handler) writeJSON(
	buffer *byteBuffer,
	jsonValue any,
	slogValue slog.Value,
	indent int,
) {
	encoder := jsoncolor.NewEncoder(buffer)

	var prefix strings.Builder
	for i := 0; i <= indent; i++ {
		prefix.WriteString("  ")
	}
	encoder.SetIndent(prefix.String(), "  ")

	if !handler.options.DisableColors {
		encoder.SetColors(&jsonColors)
	}

	if err := encoder.Encode(jsonValue); err != nil {
		buffer.writeString(slogValue.String())
		buffer.writeByte('\n')
	}
}

func (handler *Handler) writeListOrSingleElement(
	buffer *byteBuffer,
	list reflect.Value,
	indent int,
) {
	switch list.Len() {
	case 0:
		buffer.writeString(" []")
	case 1:
		value := list.Index(0)
		if value.CanInterface() {
			value = reflect.ValueOf(value.Interface())
		}

		switch value.Kind() {
		case reflect.Slice, reflect.Array:
			handler.writeListOrSingleElement(buffer, value, indent)
		case reflect.String:
			buffer.writeByte(' ')
			buffer.writeBytesWithIndentedNewlines([]byte(value.String()), indent)
		default:
			buffer.writeByte(' ')
			buffer.writeAnyWithIndentedNewlines(value, indent)
		}
	default:
		handler.writeList(buffer, list, indent)
	}
}

func (handler *Handler) writeList(buffer *byteBuffer, list reflect.Value, indent int) {
	for i := 0; i < list.Len(); i++ {
		value := list.Index(i)
		if value.CanInterface() {
			value = reflect.ValueOf(value.Interface())
		}

		switch value.Kind() {
		case reflect.Slice, reflect.Array:
			handler.writeList(buffer, value, indent+1)
		case reflect.String:
			handler.writeListItemPrefix(buffer, indent)
			buffer.writeBytesWithIndentedNewlines([]byte(value.String()), indent+1)
		default:
			handler.writeListItemPrefix(buffer, indent)
			buffer.writeAnyWithIndentedNewlines(value, indent+1)
		}
	}
}

func (handler *Handler) writeListItemPrefix(buffer *byteBuffer, indent int) {
	buffer.writeByte('\n')
	buffer.writeIndent(indent)
	handler.writeByteWithColor(buffer, '-', colorGray)
	buffer.writeByte(' ')
}

func (handler *Handler) writeLogSource(buffer *byteBuffer, programCounter uintptr) {
	frames := runtime.CallersFrames([]uintptr{programCounter})
	frame, _ := frames.Next()

	hasFunction := frame.Func != nil // May be nil for non-Go code or fully inlined functions
	hasFile := frame.File != ""      // frame.File may be blank if not known
	hasLine := frame.Line != 0       // frame.Line may be 0 if not known

	// If we have neither function nor file, we don't want to include source
	if !hasFunction && !hasFile {
		return
	}

	buffer.writeIndent(0)
	handler.writeAttributeKey(buffer, slog.SourceKey)
	buffer.writeByte(' ')

	// If we have the source function, we want to print that with file name in parentheses
	if hasFunction {
		buffer.writeString(frame.Func.Name())

		if hasFile {
			buffer.writeString(" (")
			buffer.writeString(frame.File)
			if hasLine {
				buffer.writeByte(':')
				buffer.writeDecimal(frame.Line)
			}
			buffer.writeString(")\n")
		}
		return
	}

	// If we don't have the source function, but do have the source file, we want to print that
	if hasFile {
		buffer.writeString(frame.File)
		if hasLine {
			buffer.writeByte(':')
			buffer.writeDecimal(frame.Line)
		}
		buffer.writeByte('\n')
	}
}
