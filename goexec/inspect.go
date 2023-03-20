package goexec

import (
	"context"
	"github.com/cjwomack/gonb/gonbui/protocol"
	"github.com/cjwomack/gonb/kernel"
	"github.com/pkg/errors"
	"log"
	"strings"
	"unicode/utf16"
)

// This file implements inspecting an identifier in a Cell and auto-complete functionalities.

// InspectIdentifierInCell implements an `inspect_request` from Jupyter, using `gopls`.
// It updates `main.go` with the cell contents (given as lines)
func (s *State) InspectIdentifierInCell(lines []string, skipLines map[int]bool, cursorLine, cursorCol int) (kernel.MIMEMap, error) {
	if s.gopls == nil {
		// gopls not installed.
		return make(kernel.MIMEMap), nil
	}
	if skipLines[cursorLine] {
		// Only Go sampleCellCode can be inspected here.
		return nil, errors.Errorf("goexec.InspectIdentifierInCell() can only inspect Go sampleCellCode, line %d is a secial command line: %q", cursorLine, lines[cursorLine])
	}

	// Generate `main.go` with contents of current cell.
	cursorInCell := Cursor{cursorLine, cursorCol}
	_, cursorInFile, err := s.parseLinesAndComposeMain(nil, lines, skipLines, cursorInCell)
	if err != nil {
		if errors.Is(err, ParseError) || errors.Is(err, CursorLost) {
			return make(kernel.MIMEMap), nil
		}
	}

	// Query `gopls`.
	ctx := context.Background()
	var desc string
	log.Printf("Calling gopls.Definition(ctx, %s, %d, %d)",
		s.MainPath(), cursorInFile.Line, cursorInFile.Col)
	desc, err = s.gopls.Definition(ctx, s.MainPath(), cursorInFile.Line, cursorInFile.Col)
	messages := s.gopls.ConsumeMessages()
	if err != nil {
		parts := []string{errors.Cause(err).Error()}
		if len(messages) > 0 {
			parts = append(parts, messages...)
		}
		return kernel.MIMEMap{protocol.MIMETextPlain: strings.Join(parts, "\n\n")}, nil
	}

	// Return MIMEMap with markdown.
	return kernel.MIMEMap{protocol.MIMETextMarkdown: desc}, nil
}

// AutoCompleteOptionsInCell implements an `complete_request` from Jupyter, using `gopls`.
// It updates `main.go` with the cell contents (given as lines)
func (s *State) AutoCompleteOptionsInCell(cellLines []string, skipLines map[int]bool,
	cursorLine, cursorCol int, reply *kernel.CompleteReply) (err error) {
	if s.gopls == nil {
		// gopls not installed.
		return
	}
	if skipLines[cursorLine] {
		// Only Go sampleCellCode can be inspected here.
		err = errors.Errorf("goexec.AutoCompleteOptionsInCell() can only auto-complete Go sampleCellCode, line %d is a secial command line: %q", cursorLine, cellLines[cursorLine])
		return
	}

	// Generate `main.go` with contents of current cell.
	cursorInCell := Cursor{cursorLine, cursorCol}
	var cursorInFile Cursor
	_, cursorInFile, err = s.parseLinesAndComposeMain(nil, cellLines, skipLines, cursorInCell)
	if err != nil {
		if errors.Is(err, ParseError) || errors.Is(err, CursorLost) {
			// Simply return no auto-complete.
			err = nil
		}
		return
	}

	// Query `gopls`.
	ctx := context.Background()
	_ = cursorInFile
	var matches []string
	var replaceLength int
	matches, replaceLength, err = s.gopls.Complete(ctx, s.MainPath(), cursorInFile.Line, cursorInFile.Col)
	if err != nil {
		err = errors.Cause(err)
		return
	}
	if replaceLength > 0 {
		replaceStr := cellLines[cursorLine][cursorCol-replaceLength : cursorCol]
		replaceLengthUTF16 := len(utf16.Encode([]rune(replaceStr)))
		reply.CursorStart -= replaceLengthUTF16
	}
	if len(matches) > 0 {
		reply.Matches = matches
	}
	return
}
