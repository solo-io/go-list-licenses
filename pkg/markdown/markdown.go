package markdown

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

type Writer struct {
	w *bufio.Writer
	headers []string
	headerLen int
}

func NewWriter(w io.Writer, headers []string) *Writer {
	writer := &Writer{
		w: bufio.NewWriter(w),
		headers: headers,
	}
	var sep string
	for idx, header := range headers{
		if idx > 0 {
			sep += "|"
			writer.w.WriteRune('|')
		}
		sep += "---"
		writer.w.WriteString(header)
	}
	writer.w.WriteString(fmt.Sprintf("\n%s\n", sep))
	writer.headerLen = writer.w.Buffered()
	return writer
}

// Write
func (w *Writer) Write(record []string) error {
	if len(record) != len(w.headers){
		return errors.New("incorrect amount of columns to match headers")
	}

	for idx, col := range record {
		if idx > 0 {
			if _, err := w.w.WriteRune('|'); err != nil {
				return err
			}
		}
		if _, err := w.w.WriteString(col); err != nil {
			return err
		}
	}
	err := w.w.WriteByte('\n')
	return err
}

// Flush writes any buffered data to the underlying io.Writer if any thing else other than the headers have been written.
// To check if an error occurred during the Flush, call Error.
func (w *Writer) Flush() error {
	if w.w.Buffered() != w.headerLen{
		err := w.w.Flush()
		if err != nil {
			return err
		}
	}
	return nil
}