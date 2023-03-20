package gonbui

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"github.com/gofrs/uuid"
	"github.com/cjwomack/gonb/gonbui/protocol"
	"github.com/pkg/errors"
	"image"
	"image/png"
	"os"
	"sync"
)

func init() {
	IsNotebook = os.Getenv(protocol.GONB_PIPE_ENV) != ""
}

var (
	// IsNotebook indicates whether the execution was started by a GoNB kernel.
	IsNotebook bool

	// mu control access to displaying content.
	mu sync.Mutex

	// gonbPipe is the currently opened gonbPipe, if one is opened.
	gonbPipe      *os.File
	gonbPipeError error
	gonbEncoder   *gob.Encoder
)

// Error returns the first error that may have happened in communication to the kernel. Nil if there has been
// no errors.
func Error() error {
	mu.Lock()
	defer mu.Unlock()
	return gonbPipeError
}

// openLock a singleton connection to the GoNB kernel, assuming `mu` is already locked. Returns any potential error.
// Errors can also later be accessed by the Error() function. The connection will be used by all Display* functions.
//
// Notice the display functions will call Open automatically, so it's not necessarily needed. Also, if the pipe is
// already opened, this becomes a no-op.
func openLocked() error {
	if gonbPipeError != nil {
		return gonbPipeError // Errors are persistent, it won't recover.
	}
	if gonbPipe != nil {
		// Pipe already opened.
		return nil
	}
	gonbPipe, gonbPipeError = os.OpenFile(os.Getenv(protocol.GONB_PIPE_ENV), os.O_WRONLY, 0600)
	if gonbPipeError != nil {
		gonbPipeError = errors.Wrapf(gonbPipeError, "failed opening pipe %q", os.Getenv(protocol.GONB_PIPE_ENV))
		return gonbPipeError
	}
	gonbEncoder = gob.NewEncoder(gonbPipe)
	return nil
}

// sendData to be displayed in the connected Notebook.
func sendData(data *protocol.DisplayData) {
	mu.Lock()
	defer mu.Unlock()
	if err := openLocked(); err != nil {
		return
	}
	err := gonbEncoder.Encode(data)
	if err != nil {
		gonbPipeError = errors.Wrapf(err, "failed to write to pipe %q", os.Getenv(protocol.GONB_PIPE_ENV))
		gonbPipe.Close()
	}
}

// DisplayHTML will display the given HTML in the notebook, as the output of the cell being executed.
func DisplayHTML(html string) {
	if !IsNotebook {
		return
	}
	sendData(&protocol.DisplayData{
		Data: map[protocol.MIMEType]any{protocol.MIMETextHTML: html},
	})
}

// UpdateHTML displays the given HTML in the notebook on an output block with the given `id`: the block is created
// automatically the first time this function is called, and simply updated thereafter.
//
// The contents of these cells are considered transient, and intended to live only during a kernel session.
//
// Usage example:
//
// ```go
//
//	counterDisplayId := gonbui.UniqueID()
//	for ii := 0; ii < 10; ii++ {
//	  gonbui.UpdateHTML(counterDisplayId, fmt.Sprintf("Count: <b>%d</b>\n", ii))
//	}
//	gonbui.UpdateHTML(counterDisplayId, "")  // Erase transient block.
//	gonbui.DisplayHTML(fmt.Sprintf("Count: <b>%d</b>\n", ii))  // Show on final block.
//
// ```
func UpdateHTML(id, html string) {
	if !IsNotebook {
		return
	}
	sendData(&protocol.DisplayData{
		Data:      map[protocol.MIMEType]any{protocol.MIMETextHTML: html},
		DisplayID: id,
	})
}

// UniqueID returns a unique id that can be used for UpdateHTML. Should be generated once per display block
// the program wants to update.
func UniqueID() string {
	uuid, _ := uuid.NewV7()
	uuidStr := uuid.String()
	uid := uuidStr[len(uuidStr)-8:]
	return fmt.Sprintf("gonb_id_%s", uid)
}

// DisplayPNG displays the given PNG, given as raw bytes.
func DisplayPNG(png []byte) {
	if !IsNotebook {
		return
	}
	sendData(&protocol.DisplayData{
		Data: map[protocol.MIMEType]any{protocol.MIMEImagePNG: png},
	})
}

// DisplayImage displays the given image, by converting it to PNG first.
// It returns an error if it fails to encode to the image to PNG.
func DisplayImage(image image.Image) error {
	buf := bytes.NewBuffer(nil)
	err := png.Encode(buf, image)
	if err != nil {
		return err
	}
	DisplayPNG(buf.Bytes())
	return nil
}

func DisplaySVG(svg string) {
	if !IsNotebook {
		return
	}
	// This should be the implementation, but the Jupyter doesn't handle well SVG data
	// when the notebook is converted to HTML. So we try a simple work around of embedding
	// the SVG as HTML.
	// (Question in Jupyter forum:
	// https://discourse.jupyter.org/t/svg-either-not-loading-right-or-not-exporting-to-html/17824)
	//sendData(&protocol.DisplayData{
	//	Data: map[protocol.MIMEType]any{protocol.MIMEImageSVG: svg},
	//})
	DisplayHTML(fmt.Sprintf("<div>%s</div>", svg))
}

// EmbedImageAsPNGSrc returns a string that can be used as in an HTML <img> tag, as its source (it's `src` field).
// This simplifies embedding an image in HTML without requiring separate files. It embeds it as a PNG file
// base64 encoded.
func EmbedImageAsPNGSrc(img image.Image) (string, error) {
	buf := &bytes.Buffer{}
	err := png.Encode(buf, img)
	if err != nil {
		return "", errors.Wrapf(err, "failed to encode image as PNG")
	}
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return fmt.Sprintf("data:image/png;base64,%s", encoded), nil
}
